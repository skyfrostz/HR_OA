package service

import (
	"context"
	"math"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

func normalizeEmployeeFilter(filter model.EmployeeFilter) (model.EmployeeFilter, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	if (filter.Query != "" && !validText(filter.Query, 100)) ||
		(filter.DepartmentID != nil && *filter.DepartmentID == 0) {
		return model.EmployeeFilter{}, ErrInvalid
	}
	return filter, nil
}

func employeeSearchPage(input model.EmployeeSearch) (model.Pagination, error) {
	p := model.Pagination{Page: 1, PageSize: 20}
	if input.Page != nil {
		p.Page = *input.Page
	}
	if input.PageSize != nil {
		p.PageSize = *input.PageSize
	}
	if p.Page < 1 || p.PageSize < 1 || p.PageSize > 100 || p.Page-1 > math.MaxInt/p.PageSize {
		return p, ErrInvalid
	}
	return p, nil
}

func filterEmployees(db *gorm.DB, filter model.EmployeeFilter) *gorm.DB {
	if filter.Query != "" {
		// LOCATE treats %, _, and backslashes literally. The grouped OR cannot
		// widen the employee/role scope already attached to this query.
		db = db.Where("(LOCATE(?, oa_employees.name) > 0 OR LOCATE(?, oa_employees.employee_no) > 0 OR LOCATE(?, oa_employees.department) > 0)",
			filter.Query, filter.Query, filter.Query)
	}
	if filter.DepartmentID != nil {
		db = db.Where("oa_employees.department_id = ?", *filter.DepartmentID)
	}
	return db
}

func (s *Service) SearchEmployees(ctx context.Context, actor model.Actor, input model.EmployeeSearch) (model.Page[model.EmployeeView], error) {
	p, err := employeeSearchPage(input)
	if err != nil {
		return model.Page[model.EmployeeView]{}, err
	}
	filter, err := normalizeEmployeeFilter(input.EmployeeFilter)
	if err != nil {
		return model.Page[model.EmployeeView]{}, err
	}
	return s.listFilteredEmployees(ctx, actor, p, filter)
}
