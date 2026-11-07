package service

import (
	"bytes"
	"context"
	"errors"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

type AttachmentMigrationResult struct {
	Examined    int                       `json:"examined"`
	Verified    int                       `json:"verified"`
	Migrated    int                       `json:"migrated"`
	NextAfterID uint                      `json:"nextAfterId"`
	List        []AttachmentMigrationItem `json:"list"`
}
type AttachmentMigrationItem struct {
	ID     uint   `json:"id"`
	SHA256 string `json:"sha256,omitempty"`
	Status string `json:"status"`
}

func (s *Service) MigrateAttachments(ctx context.Context, afterID uint, limit int, apply bool) (AttachmentMigrationResult, error) {
	result := AttachmentMigrationResult{NextAfterID: afterID, List: []AttachmentMigrationItem{}}
	if limit < 1 || limit > 500 {
		return result, ErrInvalid
	}
	if apply && s.oss == nil {
		return result, ErrIntegrationDisabled
	}
	var rows []model.ContractFile
	db := s.db.WithContext(ctx)
	if err := db.Where("storage_backend = ? AND id > ?", model.StorageLocal, afterID).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := AttachmentMigrationItem{ID: row.ID, Status: "SOURCE_UNAVAILABLE"}
		result.Examined++
		data, err := s.readStoredFile(ctx, row)
		if err == nil {
			item.SHA256 = fileDigest(data)
			item.Status = "LOCAL_VERIFIED"
			if apply {
				item.Status = s.migrateFile(ctx, row, data, item.SHA256)
			}
		}
		if item.Status == "LOCAL_VERIFIED" || item.Status == "MIGRATED" {
			result.Verified++
		}
		if item.Status == "MIGRATED" {
			result.Migrated++
		}
		result.NextAfterID = row.ID
		result.List = append(result.List, item)
	}
	return result, nil
}

func (s *Service) migrateFile(ctx context.Context, row model.ContractFile, data []byte, sha string) string {
	err := s.oss.Put(ctx, row.StorageKey, row.ContentType, data)
	if err != nil && !errors.Is(err, integration.ErrObjectExists) {
		return "UPLOAD_FAILED"
	}
	remote, err := s.oss.Read(ctx, row.StorageKey, row.Size)
	if err != nil || !bytes.Equal(remote, data) {
		return "VERIFY_FAILED"
	}
	result := s.db.WithContext(ctx).Model(&model.ContractFile{}).
		Where("id = ? AND storage_backend = ? AND storage_key = ? AND size = ? AND version = ?", row.ID, model.StorageLocal, row.StorageKey, row.Size, row.Version).
		Updates(map[string]any{"storage_backend": model.StorageOSS, "sha256": sha})
	if result.Error != nil || result.RowsAffected != 1 {
		return "SWITCH_CONFLICT"
	}
	return "MIGRATED"
}
