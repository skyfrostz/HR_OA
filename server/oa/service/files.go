package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxFileSize = 10 * 1024 * 1024

func (s *Service) SaveFile(ctx context.Context, actor model.Actor, contractID uint, originalName, classification string, reader io.Reader) (model.FileView, error) {
	if !actor.CanWrite() {
		return model.FileView{}, ErrForbidden
	}
	var contract model.Contract
	if err := s.visibleContracts(ctx, actor).Select("id").Take(&contract, contractID).Error; err != nil {
		return model.FileView{}, queryError(err)
	}

	originalName = strings.TrimSpace(path.Base(strings.ReplaceAll(originalName, "\\", "/")))
	if !validText(originalName, 255) || strings.ContainsFunc(originalName, unicode.IsControl) {
		return model.FileView{}, ErrInvalid
	}
	source := bufio.NewReader(reader)
	contentType, err := sniffFile(originalName, source)
	if err != nil {
		return model.FileView{}, err
	}

	if classification == "" {
		classification = model.FileStandard
	}
	if !validClassification(classification) {
		return model.FileView{}, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(source, MaxFileSize+1))
	if err != nil {
		return model.FileView{}, err
	}
	if len(data) > MaxFileSize {
		return model.FileView{}, ErrTooLarge
	}
	var randomKey [16]byte
	if _, err := rand.Read(randomKey[:]); err != nil {
		return model.FileView{}, err
	}
	key := hex.EncodeToString(randomKey[:])
	backend := model.StorageLocal
	if s.writeBackend == model.StorageOSS && s.oss != nil {
		backend = model.StorageOSS
		err = s.oss.Put(ctx, key, contentType, data)
	} else {
		err = s.putLocalFile(key, data)
	}
	if err != nil {
		return model.FileView{}, err
	}

	keepFile := false
	defer func() {
		if !keepFile && backend == model.StorageLocal {
			if root, err := os.OpenRoot(s.storageDir); err == nil {
				defer root.Close()
				_ = root.Remove(key)
			}
		}
	}()
	record := model.ContractFile{
		ContractID: contract.ID, OriginalName: originalName, Size: int64(len(data)), ContentType: contentType, StorageKey: key,
		Classification: classification, Version: 1, StorageBackend: backend, SHA256: fileDigest(data),
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return createError(err)
		}
		// A commit error can have an unknown outcome. Keep the blob if the insert reached commit.
		keepFile = true
		return nil
	})
	if err != nil {
		return model.FileView{}, err
	}
	return fileView(record), nil
}

func sniffFile(name string, source *bufio.Reader) (string, error) {
	header, err := source.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	contentType := http.DetectContentType(header)
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		if contentType == "application/pdf" {
			return contentType, nil
		}
	case ".jpg", ".jpeg":
		if contentType == "image/jpeg" {
			return contentType, nil
		}
	case ".png":
		if contentType == "image/png" {
			return contentType, nil
		}
	}
	return "", ErrInvalid
}

func writeFile(file *os.File, source io.Reader) (int64, error) {
	size, err := io.Copy(file, io.LimitReader(source, MaxFileSize+1))
	if err == nil && size > MaxFileSize {
		err = ErrTooLarge
	}
	if err == nil {
		err = file.Sync()
	}
	return size, errors.Join(err, file.Close())
}

func validClassification(value string) bool {
	return value == model.FileStandard || value == model.FileRestricted
}

func scopedFiles(db *gorm.DB, actor model.Actor) *gorm.DB {
	if !actor.CanReadSalary() {
		db = db.Where("classification = ?", model.FileStandard)
	}
	return db.Order("id ASC")
}

func (s *Service) OpenFile(ctx context.Context, actor model.Actor, id uint, preview bool) (model.FileView, io.ReadCloser, error) {
	var record model.ContractFile
	contracts := s.visibleContracts(ctx, actor).Select("oa_contracts.id")
	err := scopedFiles(s.db.WithContext(ctx), actor).Where("contract_id IN (?)", contracts).Take(&record, id).Error
	if err != nil {
		return model.FileView{}, nil, queryError(err)
	}
	data, err := s.readStoredFile(ctx, record)
	if err != nil {
		return model.FileView{}, nil, err
	}
	if preview {
		switch record.ContentType {
		case "image/jpeg", "image/png":
			cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil || "image/"+format != record.ContentType || cfg.Width < 1 || cfg.Height < 1 ||
				cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width*cfg.Height > 16*1024*1024 {
				return model.FileView{}, nil, ErrInvalid
			}
		case "application/pdf":
			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				return model.FileView{}, nil, ErrInvalid
			}
		default:
			return model.FileView{}, nil, ErrInvalid
		}
	}
	return fileView(record), io.NopCloser(bytes.NewReader(data)), nil
}

func (s *Service) UpdateClassification(ctx context.Context, actor model.Actor, id uint, input model.AttachmentClassification) (model.FileView, error) {
	var record model.ContractFile
	if !actor.CanWrite() {
		return fileView(record), ErrForbidden
	}
	reason, err := auditReason(input.Reason, "")
	if err != nil || input.Version < 1 || !validClassification(input.Classification) {
		return fileView(record), ErrInvalid
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&record, id).Error; err != nil {
			return queryError(err)
		}
		if record.Version != input.Version {
			return ErrStaleRecord
		}
		var contract model.Contract
		if err := tx.Select("id", "employee_id").Take(&contract, record.ContractID).Error; err != nil {
			return queryError(err)
		}
		before := auditFields{"fileId": record.ID, "classification": record.Classification, "version": record.Version}
		record.Classification = input.Classification
		record.Version++
		if err := tx.Model(&record).Updates(map[string]any{"classification": record.Classification, "version": record.Version}).Error; err != nil {
			return err
		}
		event := model.AuditEvent{Type: auditAttachment, Action: "UPDATE", Reason: reason,
			EmployeeID: &contract.EmployeeID, ContractID: &contract.ID}
		return writeAudit(tx, actor, event, before,
			auditFields{"fileId": record.ID, "classification": record.Classification, "version": record.Version})
	})
	return fileView(record), recordWriteError(err)
}

func fileDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (s *Service) readStoredFile(ctx context.Context, row model.ContractFile) ([]byte, error) {
	if _, err := hex.DecodeString(row.StorageKey); err != nil || len(row.StorageKey) != 32 ||
		row.Size < 1 || row.Size > MaxFileSize {
		return nil, errors.New("invalid attachment storage metadata")
	}
	var data []byte
	var err error
	switch row.StorageBackend {
	case "", model.StorageLocal:
		data, err = s.readLocalFile(row.StorageKey, row.Size)
	case model.StorageOSS:
		if s.oss == nil {
			return nil, ErrIntegrationDisabled
		}
		data, err = s.oss.Read(ctx, row.StorageKey, row.Size)
	default:
		return nil, errors.New("invalid attachment storage backend")
	}
	if err != nil {
		return nil, err
	}
	if row.SHA256 != "" && row.SHA256 != fileDigest(data) {
		return nil, errors.New("attachment integrity check failed")
	}
	return data, nil
}

func (s *Service) readLocalFile(key string, size int64) ([]byte, error) {
	root, err := os.OpenRoot(s.storageDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entry, err := root.Lstat(key)
	if err != nil || !entry.Mode().IsRegular() || entry.Size() != size {
		return nil, errors.New("attachment unavailable")
	}
	file, err := root.Open(key)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, errors.New("attachment unavailable")
	}
	return data, nil
}

func (s *Service) putLocalFile(key string, data []byte) error {
	root, err := os.OpenRoot(s.storageDir)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := writeFile(file, bytes.NewReader(data)); err != nil {
		_ = root.Remove(key)
		return err
	}
	return nil
}
