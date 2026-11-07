package service

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

const MaxFileSize = 10 * 1024 * 1024

func (s *Service) SaveFile(ctx context.Context, actor model.Actor, contractID uint, originalName string, reader io.Reader) (model.FileView, error) {
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

	root, err := os.OpenRoot(s.storageDir)
	if err != nil {
		return model.FileView{}, err
	}
	defer root.Close()
	var randomKey [16]byte
	if _, err = rand.Read(randomKey[:]); err != nil {
		return model.FileView{}, err
	}
	key := hex.EncodeToString(randomKey[:])
	stored, err := root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return model.FileView{}, err
	}
	keepFile := false
	defer func() {
		if !keepFile {
			_ = root.Remove(key)
		}
	}()
	size, err := writeFile(stored, source)
	if err != nil {
		return model.FileView{}, err
	}

	record := model.ContractFile{
		ContractID: contract.ID, OriginalName: originalName, Size: size, ContentType: contentType, StorageKey: key,
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

func (s *Service) OpenFile(ctx context.Context, actor model.Actor, id uint) (model.FileView, *os.File, error) {
	var record model.ContractFile
	contracts := s.visibleContracts(ctx, actor).Select("oa_contracts.id")
	err := s.db.WithContext(ctx).Where("contract_id IN (?)", contracts).Take(&record, id).Error
	if err != nil {
		return model.FileView{}, nil, queryError(err)
	}
	if _, err := hex.DecodeString(record.StorageKey); err != nil || len(record.StorageKey) != 32 {
		return model.FileView{}, nil, errors.New("invalid attachment storage key")
	}

	root, err := os.OpenRoot(s.storageDir)
	if err != nil {
		return model.FileView{}, nil, err
	}
	defer root.Close()
	entry, err := root.Lstat(record.StorageKey)
	if err != nil || !entry.Mode().IsRegular() || entry.Size() != record.Size {
		return model.FileView{}, nil, errors.New("attachment unavailable")
	}
	file, err := root.Open(record.StorageKey)
	return fileView(record), file, err
}
