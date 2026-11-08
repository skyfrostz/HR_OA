package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const importValidity = 30 * time.Minute

var (
	ErrImportInvalid  = errors.New("import preview is invalid")
	ErrImportConflict = errors.New("import data has changed")
	ErrPreviewExpired = errors.New("import preview expired")
)

type stagedEmployee struct {
	Row   int                 `json:"row"`
	Input model.EmployeeInput `json:"input"`
}

func (s *Service) PreviewEmployeeImport(ctx context.Context, actor model.Actor, file io.Reader) (model.ImportPreview, error) {
	if !actor.CanWrite() {
		return model.ImportPreview{}, ErrForbidden
	}
	rows, err := readEmployeeWorkbook(file)
	if err != nil {
		return model.ImportPreview{}, err
	}
	now := workflowNow()
	db := s.db.WithContext(ctx)
	staged, err := stageEmployeeRows(db, rows, now.In(businessTime))
	if err != nil {
		return model.ImportPreview{}, err
	}
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return model.ImportPreview{}, err
	}
	preview := model.ImportPreview{
		ID: hex.EncodeToString(key[:]), Status: model.ImportPending, CreatedAt: now, ExpiresAt: now.Add(importValidity),
		TotalRows: len(rows), ValidRows: len(staged), ErrorRows: len(rows) - len(staged),
		CanConfirm: len(rows) > 0 && len(rows) == len(staged), Rows: rows,
	}
	body, err := json.Marshal(preview)
	if err != nil {
		return model.ImportPreview{}, err
	}
	validated, err := json.Marshal(staged)
	if err != nil {
		return model.ImportPreview{}, err
	}
	record := model.EmployeeImport{
		ID: preview.ID, CreatorID: actor.UserID, Status: preview.Status, CreatedAt: now,
		ExpiresAt: preview.ExpiresAt, Preview: body, Validated: validated,
	}
	if err := db.Create(&record).Error; err != nil {
		return model.ImportPreview{}, err
	}
	return preview, nil
}

func (s *Service) EmployeeImportPreview(ctx context.Context, actor model.Actor, id string) (model.ImportPreview, error) {
	if !actor.CanWrite() {
		return model.ImportPreview{}, ErrForbidden
	}
	if !validPreviewID(id) {
		return model.ImportPreview{}, ErrNotFound
	}
	var record model.EmployeeImport
	err := s.db.WithContext(ctx).Omit("Validated", "Result").Take(&record, "id = ? AND creator_id = ?", id, actor.UserID).Error
	if err != nil {
		return model.ImportPreview{}, queryError(err)
	}
	preview, err := importPreview(record, workflowNow())
	if record.Status == model.ImportCommitted || preview.Status == model.ImportExpired {
		preview.Rows = []model.ImportRow{}
	}
	return preview, err
}

func lockOwnedEmployeeImport(tx *gorm.DB, actor model.Actor, id string) (model.EmployeeImport, error) {
	var record model.EmployeeImport
	var user workflowUser
	err := workflowUsers(tx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("u.id = ? AND u.authority_id = ?", actor.UserID, model.HRAdmin).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, ErrForbidden
	}
	if err != nil {
		return record, err
	}
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Take(&record, "id = ? AND creator_id = ?", id, actor.UserID).Error
	return record, queryError(err)
}

func (s *Service) DiscardEmployeeImport(ctx context.Context, actor model.Actor, id string) (model.ImportPreview, error) {
	var result model.ImportPreview
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	if !validPreviewID(id) {
		return result, ErrNotFound
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockOwnedEmployeeImport(tx, actor, id)
		if err != nil {
			return err
		}
		if record.Status != model.ImportPending && record.Status != model.ImportExpired {
			return ErrInvalidState
		}
		now := workflowNow()
		result, err = importPreview(record, now)
		if err != nil {
			return err
		}
		result.Status, result.CanConfirm, result.Rows = model.ImportExpired, false, []model.ImportRow{}
		if record.Status == model.ImportExpired {
			return nil
		}
		if now.Before(result.ExpiresAt) {
			result.ExpiresAt = now
		}
		cleaned, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Model(&record).Updates(map[string]any{
			"status": model.ImportExpired, "expires_at": result.ExpiresAt, "purged_at": now,
			"preview": cleaned, "validated": nil,
		}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return result, err
}

func (s *Service) ConfirmEmployeeImport(ctx context.Context, actor model.Actor, id string) (model.ImportResult, error) {
	if !actor.CanWrite() {
		return model.ImportResult{}, ErrForbidden
	}
	if !validPreviewID(id) {
		return model.ImportResult{}, ErrNotFound
	}
	var result model.ImportResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockOwnedEmployeeImport(tx, actor, id)
		if err != nil {
			return err
		}
		if record.Status == model.ImportCommitted {
			return json.Unmarshal(record.Result, &result)
		}
		now := workflowNow()
		if record.Status == model.ImportExpired || !now.Before(record.ExpiresAt) {
			return ErrPreviewExpired
		}
		preview, err := importPreview(record, now)
		if err != nil {
			return err
		}
		if !preview.CanConfirm {
			return ErrImportInvalid
		}
		var staged []stagedEmployee
		if err := json.Unmarshal(record.Validated, &staged); err != nil {
			return err
		}
		if len(staged) != preview.TotalRows {
			return ErrImportInvalid
		}
		employees, err := confirmableEmployees(tx, staged, now.In(businessTime))
		if err != nil {
			return err
		}
		if err := createError(tx.CreateInBatches(&employees, 100).Error); err != nil {
			if errors.Is(err, ErrConflict) {
				return ErrImportConflict
			}
			return err
		}
		result = model.ImportResult{PreviewID: id, ImportedCount: len(employees), EmployeeIDs: make([]uint, 0, len(employees))}
		for _, employee := range employees {
			result.EmployeeIDs = append(result.EmployeeIDs, employee.ID)
			if err := writeEmployeeAudit(tx, actor, "IMPORT", "确认员工导入", nil, employee); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		preview.Status, preview.CanConfirm, preview.Rows = model.ImportCommitted, false, []model.ImportRow{}
		cleaned, err := json.Marshal(preview)
		if err != nil {
			return err
		}
		return tx.Model(&record).Updates(map[string]any{
			"status": model.ImportCommitted, "result": payload, "validated": nil, "preview": cleaned,
			"confirmed_at": now, "purged_at": now,
		}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if errors.Is(recordWriteError(err), ErrStaleRecord) {
		err = ErrImportConflict
	}
	return result, err
}

func importPreview(record model.EmployeeImport, now time.Time) (model.ImportPreview, error) {
	var preview model.ImportPreview
	if err := json.Unmarshal(record.Preview, &preview); err != nil {
		return preview, err
	}
	preview.Status = record.Status
	preview.CreatedAt, preview.ExpiresAt = record.CreatedAt.UTC(), record.ExpiresAt.UTC()
	if record.Status == model.ImportPending && !now.Before(record.ExpiresAt) {
		preview.Status = model.ImportExpired
	}
	preview.CanConfirm = preview.CanConfirm && preview.Status == model.ImportPending
	return preview, nil
}

func validPreviewID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
