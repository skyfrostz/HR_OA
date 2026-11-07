package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type notificationModeKey struct{}

func (s *Service) notificationTx(tx *gorm.DB) *gorm.DB {
	return tx.WithContext(context.WithValue(tx.Statement.Context, notificationModeKey{}, s.IntegrationStatus().Notifications))
}

func outboxMode(tx *gorm.DB) string {
	if mode, ok := tx.Statement.Context.Value(notificationModeKey{}).(string); ok {
		return mode
	}
	return "mock"
}

func (s *Service) NotificationDeliveries(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.NotificationDeliveryView], error) {
	page := model.NewPage[model.NotificationDeliveryView](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	query := s.db.WithContext(ctx).Model(&model.NotificationEvent{})
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := query.Select(`id, kind, delivery_mode AS mode, attempts, created_at,
		CASE WHEN delivery_mode = 'mock' AND delivered_at IS NOT NULL THEN 'MOCK'
		     WHEN delivery_status = 'SENDING' AND available_at <= ? THEN 'UNCERTAIN'
		     WHEN delivery_status = 'SENDING' THEN 'PENDING' ELSE delivery_status END AS status`, workflowNow()).
		Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Scan(&page.List).Error
	return page, err
}

func (s *Service) RunProviderOutbox(ctx context.Context) {
	mode := s.IntegrationStatus().Notifications
	if mode == "mock" {
		return
	}
	ticker := time.NewTicker(outboxInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		for i := 0; i < outboxBatch && ctx.Err() == nil; i++ {
			if !s.deliverWeComEvent(ctx, mode) {
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

func (s *Service) deliverWeComEvent(ctx context.Context, mode string) bool {
	attempt, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var event model.NotificationEvent
	now := workflowNow()
	db := s.db.WithContext(attempt)
	if err := db.Exec(`UPDATE oa_notification_outbox SET delivery_status = 'UNCERTAIN'
		WHERE delivery_mode = ? AND delivery_status = 'SENDING' AND available_at <= ?
		ORDER BY available_at LIMIT 100`, mode, now).Error; err != nil {
		return false
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("delivery_mode = ? AND delivery_status = ? AND available_at <= ?", mode, "PENDING", now).
			Order("available_at ASC, id ASC").Take(&event).Error
		if err != nil {
			return err
		}
		event.Attempts++
		if err := tx.Model(&event).Updates(map[string]any{
			"delivery_status": "SENDING", "attempts": event.Attempts, "available_at": now.Add(30 * time.Second),
		}).Error; err != nil {
			return err
		}
		if event.ContractID == nil {
			// The OA inbox remains usable when a user has no provider
			// binding or when the external transport fails.
			return writeProviderReceipt(tx, event, "mock", now)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false
	}
	outcome := s.sendProviderEvent(attempt, event)
	// Finalize even if shutdown cancels the provider call. A crashed process
	// leaves SENDING behind and never blindly repeats an unknown delivery.
	finalize, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	return s.completeProviderEvent(finalize, event, outcome) == nil
}

func (s *Service) sendProviderEvent(ctx context.Context, event model.NotificationEvent) integration.DeliveryOutcome {
	if event.ContractID != nil {
		valid, err := s.contractEventEligible(s.db.WithContext(ctx), event)
		if err != nil {
			return integration.DeliveryOutcome{Status: "FAILED", Retry: true}
		}
		if !valid {
			return integration.DeliveryOutcome{Status: "SKIPPED"}
		}
	}
	var binding model.WeComBinding
	validUsers := s.wecomIdentity(ctx).Select("sys_users.id").Where("sys_users.id = ?", event.RecipientID)
	err := s.db.WithContext(ctx).Where("mode = ? AND corp_id = ? AND user_id IN (?)", s.wecom.Mode(), s.wecom.CorpID(), validUsers).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return integration.DeliveryOutcome{Status: "SKIPPED"}
	}
	if err != nil {
		return integration.DeliveryOutcome{Status: "FAILED", Retry: true}
	}
	title := mockTitle(event.Kind)
	if title == "" {
		return integration.DeliveryOutcome{Status: "FAILED"}
	}
	return s.wecom.Send(ctx, binding.MemberID, title, event.ID)
}

func (s *Service) completeProviderEvent(ctx context.Context, event model.NotificationEvent, outcome integration.DeliveryOutcome) error {
	now := workflowNow()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"delivery_status": outcome.Status}
		if outcome.Retry && event.Attempts < 4 {
			updates["delivery_status"] = "PENDING"
			updates["available_at"] = now.Add((5 * time.Second) << (event.Attempts - 1))
		}
		if outcome.Status == "ACCEPTED" {
			updates["delivered_at"] = now
		}
		result := tx.Model(&model.NotificationEvent{}).Where("id = ? AND delivery_status = ? AND attempts = ?", event.ID, "SENDING", event.Attempts).
			Updates(updates)
		if result.Error != nil || result.RowsAffected != 1 || outcome.Status != "ACCEPTED" {
			return result.Error
		}
		return writeProviderReceipt(tx, event, event.DeliveryMode, now)
	})
}

func writeProviderReceipt(tx *gorm.DB, event model.NotificationEvent, mode string, now time.Time) error {
	conflict := clause.OnConflict{DoNothing: true}
	if mode != "mock" {
		conflict = clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"delivery_mode"})}
	}
	return tx.Clauses(conflict).Create(&model.MockNotification{EventID: event.ID, RequestID: event.RequestID,
		ContractID: event.ContractID, RecipientID: event.RecipientID, Kind: event.Kind, Title: mockTitle(event.Kind),
		DeliveryMode: mode, CreatedAt: now}).Error
}
