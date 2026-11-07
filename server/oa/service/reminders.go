package service

import (
	"context"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

func (s *Service) ContractReminders(ctx context.Context, actor model.Actor, days int, p model.Pagination) (model.Page[model.ContractReminder], error) {
	page := model.NewPage[model.ContractReminder](p)
	today := calendarDate(time.Now().In(businessTime))
	query := s.contractReminderQuery(ctx, actor, days)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var rows []struct {
		ContractID   uint
		ContractNo   string
		EmployeeID   uint
		EmployeeNo   string
		EmployeeName string
		EndDate      time.Time
		Status       string
	}
	err := query.Select("oa_contracts.id AS contract_id, oa_contracts.contract_no, oa_contracts.employee_id, e.employee_no, e.name AS employee_name, oa_contracts.end_date, oa_contracts.status").
		Order("oa_contracts.end_date ASC, oa_contracts.id ASC").Limit(p.PageSize).Offset(p.Offset()).Scan(&rows).Error
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		page.List = append(page.List, model.ContractReminder{
			ContractID: row.ContractID, ContractNo: row.ContractNo, EmployeeID: row.EmployeeID,
			EmployeeNo: row.EmployeeNo, EmployeeName: row.EmployeeName, EndDate: row.EndDate.Format(dateLayout),
			DaysRemaining: calendarDaysBetween(today, row.EndDate), Status: row.Status,
		})
	}
	return page, nil
}

func calendarDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}

func calendarDaysBetween(from, to time.Time) int64 {
	const secondsInDay = 24 * 60 * 60
	return (calendarDate(to).Unix() - calendarDate(from).Unix()) / secondsInDay
}

func (s *Service) contractReminderQuery(ctx context.Context, actor model.Actor, days int) *gorm.DB {
	today := calendarDate(time.Now().In(businessTime))
	return contractReminderScope(s.visibleContracts(ctx, actor), today, days)
}

func contractReminderScope(query *gorm.DB, today time.Time, days int) *gorm.DB {
	return query.
		Joins("JOIN oa_employees AS e ON e.id = oa_contracts.employee_id AND e.deleted_at IS NULL").
		Where("oa_contracts.status IN ? AND oa_contracts.end_date <= ?", []string{"SIGNED", "EXPIRED"}, today.AddDate(0, 0, days)).
		Where(`NOT EXISTS (
			SELECT 1 FROM oa_contracts AS successor
			WHERE successor.previous_contract_id = oa_contracts.id AND successor.employee_id = oa_contracts.employee_id
			AND successor.deleted_at IS NULL
			AND successor.status = 'SIGNED' AND successor.end_date > oa_contracts.end_date AND successor.end_date >= ?
		)`, today)
}
