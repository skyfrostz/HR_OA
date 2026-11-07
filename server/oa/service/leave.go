package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLeaveOverlap      = errors.New("overlapping leave")
	ErrInsufficientLeave = errors.New("insufficient leave")
)

func calculateLeave(period model.LeavePeriod, kind model.LeaveType, calendar model.LeaveCalendar) (model.LeaveCalculation, error) {
	start, startErr := parseDate(period.StartDate)
	end, endErr := parseDate(period.EndDate)
	if startErr != nil || endErr != nil || start.After(end) || calendarDaysBetween(start, end) >= 366 ||
		(period.StartHalf != "AM" && period.StartHalf != "PM") || (period.EndHalf != "AM" && period.EndHalf != "PM") ||
		(start.Equal(end) && period.StartHalf == "PM" && period.EndHalf == "AM") {
		return model.LeaveCalculation{}, ErrInvalid
	}
	result := model.LeaveCalculation{LeavePeriod: period, LeaveTypeName: kind.Name, UsesBalance: kind.UsesBalance,
		LeaveTypeVersion: kind.Version, CalendarVersion: calendar.Version,
		Years: []model.LeaveYear{}, Days: []model.LeaveDay{}}
	exceptions := make(map[string]bool, len(calendar.Exceptions))
	for _, exception := range calendar.Exceptions {
		exceptions[exception.Date] = exception.Working
	}
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		text := date.Format(dateLayout)
		weekday := (int(date.Weekday())+6)%7 + 1
		working, override := exceptions[text]
		if (override && !working) || (!override && !slices.Contains(calendar.WorkingWeekdays, weekday)) {
			continue
		}
		halves := make([]string, 0, 2)
		if !date.Equal(start) || period.StartHalf == "AM" {
			halves = append(halves, "AM")
		}
		if !date.Equal(end) || period.EndHalf == "PM" {
			halves = append(halves, "PM")
		}
		result.Days = append(result.Days, model.LeaveDay{Date: text, Halves: halves})
		result.TotalHalfDays += len(halves)
		last := len(result.Years) - 1
		if last < 0 || result.Years[last].Year != date.Year() {
			result.Years = append(result.Years, model.LeaveYear{Year: date.Year(), HalfDays: len(halves)})
		} else {
			result.Years[last].HalfDays += len(halves)
		}
	}
	if result.TotalHalfDays == 0 {
		return result, ErrInvalid
	}
	return result, nil
}

func leaveConfig(db *gorm.DB, id uint) (model.LeaveType, model.LeaveCalendar, error) {
	var kind model.LeaveType
	err := db.Take(&kind, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && id == 0) {
		return kind, model.LeaveCalendar{}, ErrInvalid
	}
	if err != nil {
		return kind, model.LeaveCalendar{}, err
	}
	calendar, err := readCalendar(db)
	return kind, calendar, err
}

func lockLeaveEmployee(tx *gorm.DB, employeeID uint) error {
	var employee model.Employee
	return queryError(tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Take(&employee, employeeID).Error)
}

func (s *Service) PreviewLeave(ctx context.Context, actor model.Actor, period model.LeavePeriod) (model.LeavePreview, error) {
	var result model.LeavePreview
	if !actor.CanSubmitRequests() {
		return result, ErrForbidden
	}
	if period.LeaveTypeID == 0 {
		return result, ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kind, calendar, err := leaveConfig(tx, period.LeaveTypeID)
		if err != nil {
			return err
		}
		if !kind.Enabled {
			return ErrInvalid
		}
		result, err = prepareLeave(tx, *actor.EmployeeID, period, kind, calendar)
		return err
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return result, err
}

func prepareLeave(db *gorm.DB, employeeID uint, period model.LeavePeriod, kind model.LeaveType, calendar model.LeaveCalendar) (model.LeavePreview, error) {
	result := model.LeavePreview{Balances: []model.AnnualLeaveView{}, Issues: []string{}}
	calculation, err := calculateLeave(period, kind, calendar)
	if err != nil {
		return result, err
	}
	result.Calculation = calculation
	var slots []model.LeaveSlot
	err = db.Select("date", "half").Where("employee_id = ? AND date BETWEEN ? AND ?", employeeID, period.StartDate, period.EndDate).Find(&slots).Error
	if err != nil {
		return result, err
	}
	occupied := make(map[string]bool, len(slots))
	for _, slot := range slots {
		occupied[slot.Date.Format(dateLayout)+slot.Half] = true
	}
	overlap := false
	for _, day := range calculation.Days {
		for _, half := range day.Halves {
			overlap = overlap || occupied[day.Date+half]
		}
	}
	if overlap {
		result.Issues = append(result.Issues, "LEAVE_OVERLAP")
	}
	if kind.UsesBalance {
		short := false
		for _, allocation := range calculation.Years {
			balance, err := readLeaveBalance(db, employeeID, allocation.Year)
			if err != nil {
				return result, err
			}
			result.Balances = append(result.Balances, balance)
			short = short || balance.RemainingHalfDays < allocation.HalfDays
		}
		if short {
			result.Issues = append(result.Issues, "INSUFFICIENT_LEAVE")
		}
	}
	result.CanSubmit = len(result.Issues) == 0
	return result, nil
}

func prepareLeaveSubmission(tx *gorm.DB, employeeID uint, form leaveForm) (*model.LeaveCalculation, error) {
	if form.LeaveTypeID == 0 || form.LeaveTypeVersion < 1 || form.CalendarVersion < 1 {
		return nil, ErrInvalid
	}
	kind, calendar, err := leaveConfig(tx, form.LeaveTypeID)
	if err != nil {
		return nil, err
	}
	if kind.Version != form.LeaveTypeVersion || calendar.Version != form.CalendarVersion {
		return nil, ErrStaleRecord
	}
	if !kind.Enabled {
		return nil, ErrInvalid
	}
	if err := lockLeaveEmployee(tx, employeeID); err != nil {
		return nil, err
	}
	preview, err := prepareLeave(tx, employeeID, form.LeavePeriod, kind, calendar)
	if err != nil {
		return nil, err
	}
	if slices.Contains(preview.Issues, "LEAVE_OVERLAP") {
		return nil, ErrLeaveOverlap
	}
	if !preview.CanSubmit {
		return nil, ErrInsufficientLeave
	}
	return &preview.Calculation, nil
}

func reserveLeave(tx *gorm.DB, request model.ApprovalRequest) error {
	if request.LeaveCalculation == nil {
		return nil
	}
	slots := make([]model.LeaveSlot, 0, request.LeaveCalculation.TotalHalfDays)
	for _, day := range request.LeaveCalculation.Days {
		date, err := parseDate(day.Date)
		if err != nil {
			return err
		}
		for _, half := range day.Halves {
			slots = append(slots, model.LeaveSlot{EmployeeID: *request.LeaveEmployeeID, Date: date, Half: half, RequestID: request.ID})
		}
	}
	if err := createError(tx.CreateInBatches(&slots, 100).Error); err != nil {
		if errors.Is(err, ErrConflict) {
			return ErrLeaveOverlap
		}
		return err
	}
	return appendLeaveLedger(tx, request, model.LeaveReserve)
}

func applyLeaveTransition(tx *gorm.DB, request model.ApprovalRequest, kind string) error {
	if request.LeaveCalculation == nil {
		return nil
	}
	if request.LeaveEmployeeID == nil {
		return ErrInvalidState
	}
	if err := lockLeaveEmployee(tx, *request.LeaveEmployeeID); err != nil {
		return err
	}
	if kind == model.LeaveRelease || kind == model.LeaveReverse {
		if err := tx.Delete(&model.LeaveSlot{}, "request_id = ?", request.ID).Error; err != nil {
			return err
		}
	}
	return appendLeaveLedger(tx, request, kind)
}

func appendLeaveLedger(tx *gorm.DB, request model.ApprovalRequest, kind string) error {
	if !request.LeaveCalculation.UsesBalance {
		return nil
	}
	entries := make([]model.LeaveLedger, 0, len(request.LeaveCalculation.Years))
	for _, item := range request.LeaveCalculation.Years {
		entries = append(entries, model.LeaveLedger{EmployeeID: *request.LeaveEmployeeID, RequestID: request.ID,
			Year: item.Year, Kind: kind, HalfDays: item.HalfDays})
	}
	return tx.Create(&entries).Error
}

type leaveTotals struct {
	Reserved int
	Used     int
}

func readLeaveTotals(db *gorm.DB, employeeID uint, year int) (leaveTotals, error) {
	var result leaveTotals
	err := db.Model(&model.LeaveLedger{}).Where("employee_id = ? AND `year` = ?", employeeID, year).
		Select(`COALESCE(SUM(CASE WHEN kind='RESERVE' THEN half_days WHEN kind IN ('CONSUME','RELEASE') THEN -half_days ELSE 0 END),0) AS reserved,
		        COALESCE(SUM(CASE WHEN kind='CONSUME' THEN half_days WHEN kind='REVERSE' THEN -half_days ELSE 0 END),0) AS used`).
		Scan(&result).Error
	return result, err
}

func addLeaveTotals(view model.AnnualLeaveView, totals leaveTotals) model.AnnualLeaveView {
	view.ReservedHalfDays, view.ApprovalUsedHalfDays = totals.Reserved, totals.Used
	view.RemainingHalfDays -= totals.Reserved + totals.Used
	return view
}

func readLeaveBalance(db *gorm.DB, employeeID uint, year int) (model.AnnualLeaveView, error) {
	row := model.AnnualLeave{EmployeeID: employeeID, Year: year}
	if err := db.Take(&row, "employee_id = ? AND `year` = ?", employeeID, year).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AnnualLeaveView{}, err
	}
	totals, err := readLeaveTotals(db, employeeID, year)
	return addLeaveTotals(annualLeaveView(row), totals), err
}

func (s *Service) LeaveLedger(ctx context.Context, actor model.Actor, employeeID uint, year int, p model.Pagination) (model.Page[model.LeaveLedger], error) {
	page := model.NewPage[model.LeaveLedger](p)
	if err := s.checkVisibleEmployee(ctx, actor, employeeID); err != nil {
		return page, err
	}
	query := s.db.WithContext(ctx).Model(&model.LeaveLedger{}).Where("employee_id = ?", employeeID)
	if year != 0 {
		query = query.Where("`year` = ?", year)
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := query.Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&page.List).Error
	return page, err
}
