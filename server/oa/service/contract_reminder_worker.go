package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const contractReminderKind = "CONTRACT_EXPIRING"

func scheduledContracts(db *gorm.DB, days int) *gorm.DB {
	today := calendarDate(time.Now().In(businessTime))
	return contractReminderScope(db.Model(&model.Contract{}), today, days).
		Where("oa_contracts.status = ? AND oa_contracts.end_date >= ? AND e.employment_status = ?", "SIGNED", today, model.EmploymentActive)
}

func (s *Service) contractEventEligible(db *gorm.DB, event model.NotificationEvent) (bool, error) {
	if event.ReminderDays != s.contractReminderDays || s.contractReminderDays == 0 {
		return false, nil
	}
	query := scheduledContracts(db, event.ReminderDays).Where("oa_contracts.id = ? AND oa_contracts.version = ?", *event.ContractID, event.ContractVersion)
	if event.DeliveryMode != "mock" {
		query = query.Where("e.user_id = ?", event.RecipientID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil || count != 1 {
		return false, err
	}
	if event.DeliveryMode != "mock" {
		return true, nil
	}
	err := workflowUsers(db).Where("u.id = ? AND u.authority_id IN ?", event.RecipientID, []uint{model.HRAdmin, model.HRViewer}).Count(&count).Error
	return count == 1, err
}

func (s *Service) RunContractReminders(ctx context.Context) {
	if s.contractReminderDays == 0 {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = s.enqueueContractReminders(attempt)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) enqueueContractReminders(ctx context.Context) error {
	db := s.db.WithContext(ctx)
	var hr []uint
	if err := workflowUsers(db).Select("u.id").Where("u.authority_id IN ?", []uint{model.HRAdmin, model.HRViewer}).Pluck("u.id", &hr).Error; err != nil {
		return err
	}
	mode := s.IntegrationStatus().Notifications
	now := workflowNow()
	var afterID uint
	for ctx.Err() == nil {
		var rows []struct {
			ID      uint
			Version int
			UserID  *uint
		}
		err := scheduledContracts(db, s.contractReminderDays).
			Select("oa_contracts.id, oa_contracts.version, e.user_id").
			Where("oa_contracts.id > ?", afterID).Order("oa_contracts.id ASC").Limit(100).Scan(&rows).Error
		if err != nil || len(rows) == 0 {
			return err
		}
		for _, row := range rows {
			var events []model.NotificationEvent
			add := func(recipient uint, delivery string) {
				key := fmt.Sprintf("contract:%d:%d:%d:%d", row.ID, row.Version, s.contractReminderDays, recipient)
				events = append(events, model.NotificationEvent{
					ContractID: &row.ID, ContractVersion: row.Version, ReminderDays: s.contractReminderDays, ScheduleKey: &key,
					RecipientID: recipient, Kind: contractReminderKind, DeliveryMode: delivery,
					DeliveryStatus: "PENDING", AvailableAt: now, CreatedAt: now,
				})
			}
			for _, recipient := range hr {
				add(recipient, "mock")
			}
			if mode != "mock" && row.UserID != nil {
				var count int64
				if err := s.wecomIdentity(ctx).Where("sys_users.id = ?", *row.UserID).Count(&count).Error; err != nil {
					return err
				}
				if count == 1 {
					add(*row.UserID, mode)
				}
			}
			if len(events) != 0 {
				if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(events, 100).Error; err != nil {
					return err
				}
			}
			afterID = row.ID
		}
	}
	return ctx.Err()
}
