package service

import (
	"context"
	"errors"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("invalid credentials")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("duplicate identifier")
	ErrTooLarge     = errors.New("upload too large")
)

type Service struct {
	db         *gorm.DB
	storageDir string
	signingKey []byte
	dummyHash  []byte
}

func New(db *gorm.DB, storageDir string, signingKey, dummyHash []byte) *Service {
	return &Service{db: db, storageDir: storageDir, signingKey: signingKey, dummyHash: dummyHash}
}

func (s *Service) Health(ctx context.Context) error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}

func queryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func createError(err error) error {
	var mysqlError *driver.MySQLError
	if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &mysqlError) && mysqlError.Number == 1062) {
		return ErrConflict
	}
	return err
}
