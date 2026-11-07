package service

import (
	"context"
	"errors"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
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
	db                   *gorm.DB
	storageDir           string
	signingKey           []byte
	dummyHash            []byte
	wecom                *integration.WeCom
	oss                  *integration.OSS
	writeBackend         string
	contractReminderDays int
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

func (s *Service) ConfigureIntegrations(wecom *integration.WeCom, oss *integration.OSS, writeBackend string, reminderDays int) {
	s.wecom, s.oss, s.writeBackend, s.contractReminderDays = wecom, oss, writeBackend, reminderDays
}

func (s *Service) IntegrationStatus() model.IntegrationStatus {
	status := model.IntegrationStatus{WeComLogin: "disabled", Notifications: "mock", AttachmentStorage: "local"}
	if s.wecom != nil {
		status.WeComLogin = "configured"
		if s.wecom.Mode() == "mock" {
			status.WeComLogin = "mock"
		}
		status.Notifications = s.wecom.DeliveryMode()
	}
	if s.writeBackend == model.StorageOSS && s.oss != nil {
		status.AttachmentStorage = "oss"
		if s.oss.Mock {
			status.AttachmentStorage = "oss-mock"
		}
	}
	return status
}
