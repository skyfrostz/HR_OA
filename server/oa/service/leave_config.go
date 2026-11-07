package service

import (
	"context"
	"slices"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

func (s *Service) LeaveTypes(ctx context.Context, actor model.Actor) (model.LeaveTypes, error) {
	result := model.LeaveTypes{List: []model.LeaveType{}}
	query := s.db.WithContext(ctx)
	if !actor.CanWrite() {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("id ASC").Find(&result.List).Error
	return result, err
}

func (s *Service) WriteLeaveType(ctx context.Context, actor model.Actor, id uint, input model.LeaveTypeInput) (model.LeaveType, error) {
	row := model.LeaveType{Name: strings.TrimSpace(input.Name), Version: 1}
	if !actor.CanWrite() {
		return row, ErrForbidden
	}
	if !validText(row.Name, 60) || input.UsesBalance == nil || input.Enabled == nil ||
		(id == 0 && input.Version != 0) || (id != 0 && input.Version < 1) {
		return row, ErrInvalid
	}
	row.UsesBalance, row.Enabled = *input.UsesBalance, *input.Enabled
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		event := model.AuditEvent{Type: auditLeaveType, Action: "CREATE", Reason: "设置请假类型"}
		var before auditFields
		if id == 0 {
			if err := createError(tx.Create(&row).Error); err != nil {
				return err
			}
		} else {
			var old model.LeaveType
			if err := tx.Take(&old, id).Error; err != nil {
				return queryError(err)
			}
			if old.Version != input.Version {
				return ErrStaleRecord
			}
			row.ID, row.Version = id, old.Version+1
			event.Action, before = "UPDATE", leaveTypeFields(old)
			if err := createError(tx.Save(&row).Error); err != nil {
				return err
			}
		}
		return writeAudit(tx, actor, event, before, leaveTypeFields(row))
	})
	return row, recordWriteError(err)
}

func leaveTypeFields(row model.LeaveType) auditFields {
	return auditFields{"id": row.ID, "name": row.Name, "enabled": row.Enabled, "usesBalance": row.UsesBalance, "version": row.Version}
}

func readCalendar(db *gorm.DB) (model.LeaveCalendar, error) {
	var calendar model.LeaveCalendar
	err := db.Take(&calendar, 1).Error
	return calendar, err
}

func (s *Service) LeaveCalendar(ctx context.Context, actor model.Actor) (model.LeaveCalendar, error) {
	if !actor.CanWrite() {
		return model.LeaveCalendar{}, ErrForbidden
	}
	return readCalendar(s.db.WithContext(ctx))
}

func calendarFromInput(input model.CalendarInput) (model.LeaveCalendar, error) {
	if input.Version < 1 || len(input.WorkingWeekdays) < 1 || len(input.WorkingWeekdays) > 7 ||
		input.Exceptions == nil || len(input.Exceptions) > 1000 {
		return model.LeaveCalendar{}, ErrInvalid
	}
	row := model.LeaveCalendar{ID: 1, Version: input.Version + 1,
		WorkingWeekdays: slices.Clone(input.WorkingWeekdays), Exceptions: make([]model.CalendarException, 0, len(input.Exceptions))}
	slices.Sort(row.WorkingWeekdays)
	if row.WorkingWeekdays[0] < 1 || row.WorkingWeekdays[len(row.WorkingWeekdays)-1] > 7 ||
		len(slices.Compact(slices.Clone(row.WorkingWeekdays))) != len(row.WorkingWeekdays) {
		return model.LeaveCalendar{}, ErrInvalid
	}
	seen := make(map[string]bool, len(input.Exceptions))
	for _, exception := range input.Exceptions {
		if _, err := parseDate(exception.Date); err != nil || exception.Working == nil || seen[exception.Date] {
			return model.LeaveCalendar{}, ErrInvalid
		}
		seen[exception.Date] = true
		row.Exceptions = append(row.Exceptions, model.CalendarException{Date: exception.Date, Working: *exception.Working})
	}
	slices.SortFunc(row.Exceptions, func(a, b model.CalendarException) int { return strings.Compare(a.Date, b.Date) })
	return row, nil
}

func (s *Service) WriteLeaveCalendar(ctx context.Context, actor model.Actor, input model.CalendarInput) (model.LeaveCalendar, error) {
	if !actor.CanWrite() {
		return model.LeaveCalendar{}, ErrForbidden
	}
	row, err := calendarFromInput(input)
	if err != nil {
		return row, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		before, err := readCalendar(tx)
		if err != nil {
			return err
		}
		if before.Version != input.Version {
			return ErrStaleRecord
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		event := model.AuditEvent{Type: auditLeaveCalendar, Action: "UPDATE", Reason: "设置工作日历"}
		return writeAudit(tx, actor, event, calendarFields(before), calendarFields(row))
	})
	return row, recordWriteError(err)
}

func calendarFields(row model.LeaveCalendar) auditFields {
	return auditFields{"version": row.Version, "workingWeekdays": row.WorkingWeekdays, "exceptions": row.Exceptions}
}
