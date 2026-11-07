package service

import (
	"context"
	"errors"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) AnnualLeave(ctx context.Context, actor model.Actor, employeeID uint, p model.Pagination) (model.Page[model.AnnualLeaveView], error) {
	page := model.NewPage[model.AnnualLeaveView](p)
	if err := s.checkVisibleEmployee(ctx, actor, employeeID); err != nil {
		return page, err
	}
	query := s.db.WithContext(ctx).Model(&model.AnnualLeave{}).Where("employee_id = ?", employeeID)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var rows []model.AnnualLeave
	if err := query.Order("`year` DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&rows).Error; err != nil {
		return page, err
	}
	for _, row := range rows {
		page.List = append(page.List, annualLeaveView(row))
	}
	return page, nil
}

func (s *Service) WriteAnnualLeave(ctx context.Context, actor model.Actor, employeeID uint, year int, input model.AnnualLeaveInput) (model.AnnualLeaveView, error) {
	if !actor.CanWrite() {
		return model.AnnualLeaveView{}, ErrForbidden
	}
	row, err := annualLeaveFromInput(employeeID, year, input)
	if err != nil {
		return model.AnnualLeaveView{}, err
	}
	if err := s.checkVisibleEmployee(ctx, actor, employeeID); err != nil {
		return model.AnnualLeaveView{}, err
	}
	reason, err := auditReason(input.Reason, "设置年假台账")
	if err != nil {
		return model.AnnualLeaveView{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		event := model.AuditEvent{Type: model.AuditAnnualLeave, EmployeeID: &employeeID, Reason: reason}
		if *input.Version == 0 {
			if err := createError(tx.Create(&row).Error); errors.Is(err, ErrConflict) {
				return ErrStaleRecord
			} else if err != nil {
				return err
			}
			event.Action = "CREATE"
			return writeAudit(tx, actor, event, nil, leaveAuditFields(row))
		}
		var before model.AnnualLeave
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&before, "employee_id = ? AND `year` = ? AND version = ?", employeeID, year, *input.Version).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStaleRecord
			}
			return err
		}
		updated := tx.Model(&model.AnnualLeave{}).
			Where("employee_id = ? AND `year` = ? AND version = ?", employeeID, year, *input.Version).
			Updates(map[string]any{
				"entitled_half_days": row.EntitledHalfDays, "used_half_days": row.UsedHalfDays,
				"note": row.Note, "version": row.Version, "updated_at": row.UpdatedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrStaleRecord
		}
		if err := tx.Take(&row, "employee_id = ? AND `year` = ?", employeeID, year).Error; err != nil {
			return err
		}
		event.Action = "UPDATE"
		return writeAudit(tx, actor, event, leaveAuditFields(before), leaveAuditFields(row))
	})
	return annualLeaveView(row), recordWriteError(err)
}

func annualLeaveFromInput(employeeID uint, year int, input model.AnnualLeaveInput) (model.AnnualLeave, error) {
	note := strings.TrimSpace(input.Note)
	if year < 2000 || year > 2100 || input.Version == nil || *input.Version < 0 ||
		input.EntitledHalfDays == nil || input.UsedHalfDays == nil ||
		*input.EntitledHalfDays < 0 || *input.EntitledHalfDays > 732 ||
		*input.UsedHalfDays < 0 || *input.UsedHalfDays > *input.EntitledHalfDays ||
		!workflowText(note, 0, 1000) {
		return model.AnnualLeave{}, ErrInvalid
	}
	return model.AnnualLeave{
		EmployeeID: employeeID, Year: year, EntitledHalfDays: *input.EntitledHalfDays, UsedHalfDays: *input.UsedHalfDays,
		Note: note, Version: *input.Version + 1, UpdatedAt: workflowNow(),
	}, nil
}

func (s *Service) checkVisibleEmployee(ctx context.Context, actor model.Actor, id uint) error {
	var employee model.Employee
	return queryError(s.visibleEmployees(ctx, actor).Select("id").Take(&employee, id).Error)
}

func annualLeaveView(row model.AnnualLeave) model.AnnualLeaveView {
	return model.AnnualLeaveView{
		ID: row.ID, EmployeeID: row.EmployeeID, Year: row.Year, EntitledHalfDays: row.EntitledHalfDays,
		UsedHalfDays: row.UsedHalfDays, RemainingHalfDays: row.EntitledHalfDays - row.UsedHalfDays,
		Note: row.Note, Version: row.Version, UpdatedAt: row.UpdatedAt.UTC(),
	}
}
