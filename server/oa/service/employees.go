package service

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var businessTime = time.FixedZone("CST", 8*60*60)

const (
	dateLayout     = "2006-01-02"
	maxSalaryCents = 1<<53 - 1
)

func (s *Service) visibleEmployees(ctx context.Context, actor model.Actor) *gorm.DB {
	db := s.db.WithContext(ctx).Model(&model.Employee{})
	switch actor.AuthorityID {
	case model.HRAdmin, model.HRViewer:
		return db
	case model.EmployeeRole, model.ApproverRole:
		if actor.EmployeeID != nil {
			return db.Where("oa_employees.id = ? AND oa_employees.user_id = ?", *actor.EmployeeID, actor.UserID)
		}
	}
	return db.Where("1 = 0")
}

func (s *Service) employeeProjection(ctx context.Context, actor model.Actor) *gorm.DB {
	db := s.visibleEmployees(ctx, actor)
	if !actor.CanReadSalary() {
		db = db.Omit("SalaryCents")
	}
	return db
}

func (s *Service) ListEmployees(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.EmployeeView], error) {
	return s.listFilteredEmployees(ctx, actor, p, model.EmployeeFilter{})
}

func (s *Service) listFilteredEmployees(ctx context.Context, actor model.Actor, p model.Pagination, filter model.EmployeeFilter) (model.Page[model.EmployeeView], error) {
	page := model.NewPage[model.EmployeeView](p)
	if err := filterEmployees(s.visibleEmployees(ctx, actor), filter).Count(&page.Total).Error; err != nil {
		return page, err
	}
	var employees []model.Employee
	err := filterEmployees(s.employeeProjection(ctx, actor), filter).Order("id ASC").Limit(p.PageSize).Offset(p.Offset()).Find(&employees).Error
	now := time.Now().In(businessTime)
	for _, employee := range employees {
		page.List = append(page.List, employeeView(employee, actor.CanReadSalary(), now))
	}
	return page, err
}

func (s *Service) Employee(ctx context.Context, actor model.Actor, id uint) (model.EmployeeView, error) {
	var employee model.Employee
	if err := s.employeeProjection(ctx, actor).Take(&employee, id).Error; err != nil {
		return model.EmployeeView{}, queryError(err)
	}
	return employeeView(employee, actor.CanReadSalary(), time.Now().In(businessTime)), nil
}

func (s *Service) CreateEmployee(ctx context.Context, actor model.Actor, input model.EmployeeInput) (model.EmployeeView, error) {
	if !actor.CanWrite() {
		return model.EmployeeView{}, ErrForbidden
	}

	now := time.Now().In(businessTime)
	employee, err := employeeFromInput(input, now)
	if err != nil {
		return model.EmployeeView{}, err
	}
	reason, err := auditReason(input.Reason, "新增员工档案")
	if err != nil {
		return model.EmployeeView{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := resolveEmployeeDepartment(tx, actor, reason, &employee); err != nil {
			return err
		}
		if err := createError(tx.Create(&employee).Error); err != nil {
			return err
		}
		return writeEmployeeAudit(tx, actor, "CREATE", reason, nil, employee)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return model.EmployeeView{}, recordWriteError(err)
	}
	return employeeView(employee, true, now), nil
}

func (s *Service) EditEmployee(ctx context.Context, actor model.Actor, id uint, input model.EmployeeEdit) (model.EmployeeView, error) {
	if !actor.CanWrite() {
		return model.EmployeeView{}, ErrForbidden
	}
	if input.Version < 1 {
		return model.EmployeeView{}, ErrInvalid
	}
	now := time.Now().In(businessTime)
	employee, err := employeeFromInput(input.EmployeeInput, now)
	if err != nil {
		return model.EmployeeView{}, err
	}
	reason, err := auditReason(input.Reason, "更新员工档案")
	if err != nil {
		return model.EmployeeView{}, err
	}
	employee.ID, employee.Version = id, input.Version+1
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := resolveEmployeeDepartment(tx, actor, reason, &employee); err != nil {
			return err
		}
		var before model.Employee
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&before, id).Error; err != nil {
			return queryError(err)
		}
		if before.Version != input.Version {
			return ErrStaleRecord
		}
		employee.EmploymentStatus = before.EmploymentStatus
		updated := tx.Model(&model.Employee{}).Where("id = ? AND version = ?", id, input.Version).
			Updates(map[string]any{
				"employee_no": employee.EmployeeNo, "name": employee.Name, "birth_date": employee.BirthDate,
				"hire_date": employee.HireDate, "department": employee.Department, "department_id": employee.DepartmentID,
				"salary_cents": employee.SalaryCents, "version": employee.Version,
			})
		if updated.Error != nil {
			return createError(updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrStaleRecord
		}
		return writeEmployeeAudit(tx, actor, "UPDATE", reason, &before, employee)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return model.EmployeeView{}, recordWriteError(err)
	}
	return employeeView(employee, true, now), nil
}

func employeeFromInput(input model.EmployeeInput, now time.Time) (model.Employee, error) {
	number := strings.TrimSpace(input.EmployeeNo)
	name := strings.TrimSpace(input.Name)
	department := strings.TrimSpace(input.Department)
	birth, birthErr := parseDate(input.BirthDate)
	hire, hireErr := parseDate(input.HireDate)
	if !validText(number, 64) || !validText(name, 128) ||
		(input.DepartmentID == nil && department == "") || (department != "" && !validText(department, 128)) ||
		(input.DepartmentID != nil && *input.DepartmentID == 0) ||
		birthErr != nil || hireErr != nil || hire.Before(birth) || input.BirthDate > now.Format(dateLayout) ||
		input.SalaryCents == nil || *input.SalaryCents < 0 || *input.SalaryCents > maxSalaryCents {
		return model.Employee{}, ErrInvalid
	}

	return model.Employee{
		EmployeeNo: number, Name: name, Department: department, DepartmentID: input.DepartmentID,
		BirthDate: birth, HireDate: hire, SalaryCents: *input.SalaryCents, Version: 1, EmploymentStatus: model.EmploymentActive,
	}, nil
}

func validText(value string, max int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= max &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func parseDate(value string) (time.Time, error) {
	date, err := time.Parse(dateLayout, value)
	if err != nil || date.Year() < 1000 || date.Format(dateLayout) != value {
		return time.Time{}, ErrInvalid
	}
	return date, nil
}

func employeeView(employee model.Employee, salaryAllowed bool, now time.Time) model.EmployeeView {
	view := model.EmployeeView{
		ID: employee.ID, EmployeeNo: employee.EmployeeNo, Name: employee.Name, Department: employee.Department,
		DepartmentID: employee.DepartmentID, Version: employee.Version, EmploymentStatus: employee.EmploymentStatus,
		BirthDate: employee.BirthDate.Format(dateLayout), HireDate: employee.HireDate.Format(dateLayout),
		Age: completedYears(employee.BirthDate, now), TenureYears: completedYears(employee.HireDate, now),
	}
	if salaryAllowed {
		view.SalaryCents = &employee.SalaryCents
	}
	return view
}

func completedYears(start, now time.Time) int {
	years := now.Year() - start.Year()
	if now.Month() < start.Month() || (now.Month() == start.Month() && now.Day() < start.Day()) {
		years--
	}
	return max(years, 0)
}
