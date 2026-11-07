package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	outboxInterval = 250 * time.Millisecond
	outboxBatch    = 16
)

// RunMockOutbox blocks until canceled. Each event uses a short MySQL transaction.
// The mock adapter writes an inbox receipt and performs no external I/O.
func (s *Service) RunMockOutbox(ctx context.Context, failDelivery bool) {
	ticker := time.NewTicker(outboxInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		for i := 0; i < outboxBatch && ctx.Err() == nil; i++ {
			attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
			handled, err := s.deliverNextMock(attempt, failDelivery)
			cancel()
			if err != nil || !handled {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) deliverNextMock(ctx context.Context, failDelivery bool) (bool, error) {
	handled := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := workflowNow()
		var event model.NotificationEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("delivered_at IS NULL AND available_at <= ?", now).
			Order("available_at ASC, id ASC").Take(&event).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		handled = true
		update := tx.Model(&event)
		title := mockTitle(event.Kind)
		if failDelivery || title == "" {
			return update.Updates(map[string]any{
				"attempts": gorm.Expr("attempts + 1"), "available_at": now.Add(mockBackoff(event.Attempts)),
			}).Error
		}
		receipt := model.MockNotification{
			EventID: event.ID, RequestID: event.RequestID, RecipientID: event.RecipientID,
			Kind: event.Kind, Title: title, CreatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error; err != nil {
			return err
		}
		return update.Updates(map[string]any{
			"attempts": gorm.Expr("attempts + 1"), "delivered_at": now,
		}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return handled, err
}

func mockBackoff(attempts uint) time.Duration {
	return (1 << min(attempts, 3)) * time.Second
}

func mockTitle(kind string) string {
	switch kind {
	case model.TaskAssigned:
		return "有新的待办申请"
	case model.RequestApproved:
		return "申请已通过"
	case model.RequestRejected:
		return "申请已驳回"
	default:
		return ""
	}
}
