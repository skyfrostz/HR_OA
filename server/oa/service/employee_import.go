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
	return importPreview(record, time.Now())
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
		var user workflowUser
		err := workflowUsers(tx).Clauses(clause.Locking{Strength: "SHARE"}).
			Where("u.id = ? AND u.authority_id = ?", actor.UserID, model.HRAdmin).Take(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		var record model.EmployeeImport
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&record, "id = ? AND creator_id = ?", id, actor.UserID).Error; err != nil {
			return queryError(err)
		}
		if record.Status == model.ImportCommitted {
			return json.Unmarshal(record.Result, &result)
		}
		now := workflowNow()
		if !now.Before(record.ExpiresAt) {
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
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Model(&record).Updates(map[string]any{
			"status": model.ImportCommitted, "result": payload, "validated": nil,
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
	preview.CanConfirm = preview.CanConfirm && record.Status == model.ImportPending && now.Before(record.ExpiresAt)
	return preview, nil
}

func validPreviewID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
