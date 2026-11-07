package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrStaleRecord = errors.New("stale record")

func (s *Service) Departments(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.DepartmentView], error) {
	page := model.NewPage[model.DepartmentView](p)
	if actor.AuthorityID != model.HRAdmin && actor.AuthorityID != model.HRViewer {
		return page, ErrForbidden
	}
	query := s.db.WithContext(ctx).Model(&model.Department{})
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var rows []model.Department
	if err := query.Order("id ASC").Limit(p.PageSize).Offset(p.Offset()).Find(&rows).Error; err != nil {
		return page, err
	}
	for _, row := range rows {
		page.List = append(page.List, departmentView(row))
	}
	return page, nil
}

func (s *Service) WriteDepartment(ctx context.Context, actor model.Actor, id uint, input model.DepartmentEdit) (model.DepartmentView, error) {
	if !actor.CanWrite() {
		return model.DepartmentView{}, ErrForbidden
	}
	name := strings.TrimSpace(input.Name)
	if !validText(name, 128) || (id != 0 && input.Version < 1) || (input.ParentID != nil && *input.ParentID == 0) {
		return model.DepartmentView{}, ErrInvalid
	}

	reason, err := auditReason(input.Reason, "维护部门")
	if err != nil {
		return model.DepartmentView{}, err
	}
	var row model.Department
	// The repeatable-read locking scan serializes topology changes, including
	// concurrent changes on different nodes that together would form a cycle.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var departments []model.Department
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&departments).Error; err != nil {
			return err
		}
		parents := make(map[uint]*uint, len(departments)+1)
		for _, department := range departments {
			parents[department.ID] = department.ParentID
			if department.ID == id {
				row = department
			}
		}
		if id != 0 && row.ID == 0 {
			return ErrNotFound
		}
		if id != 0 && row.Version != input.Version {
			return ErrStaleRecord
		}
		parents[id] = input.ParentID
		if !validDepartmentTree(parents) {
			return ErrInvalid
		}
		if id == 0 {
			row = model.Department{Name: name, ParentID: input.ParentID, Version: 1}
			if err := createError(tx.Create(&row).Error); err != nil {
				return err
			}
			return writeDepartmentAudit(tx, actor, "CREATE", reason, nil, row)
		}
		before := row
		renamed := row.Name != name
		row.Name, row.ParentID, row.Version = name, input.ParentID, row.Version+1
		if err := tx.Model(&row).Updates(map[string]any{"name": name, "parent_id": row.ParentID, "version": row.Version}).Error; err != nil {
			return createError(err)
		}
		if renamed {
			if err := tx.Model(&model.Employee{}).Where("department_id = ?", id).
				Updates(map[string]any{"department": name, "version": gorm.Expr("version + 1")}).Error; err != nil {
				return err
			}
		}
		return writeDepartmentAudit(tx, actor, "UPDATE", reason, &before, row)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return departmentView(row), recordWriteError(err)
}

func validDepartmentTree(parents map[uint]*uint) bool {
	for node := range parents {
		for depth := 1; ; depth++ {
			parent, exists := parents[node]
			if !exists || depth > 20 {
				return false
			}
			if parent == nil {
				break
			}
			node = *parent
		}
	}
	return true
}

func departmentView(row model.Department) model.DepartmentView {
	return model.DepartmentView{ID: row.ID, Name: row.Name, ParentID: row.ParentID, Version: row.Version}
}

// resolveEmployeeDepartment keeps the legacy free-text create path and takes
// a shared lock so a concurrent rename cannot commit a stale employee label.
func resolveEmployeeDepartment(tx *gorm.DB, actor model.Actor, reason string, employee *model.Employee) error {
	shared := clause.Locking{Strength: "SHARE"}
	var row model.Department
	if employee.DepartmentID != nil {
		err := tx.Clauses(shared).Take(&row, *employee.DepartmentID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		if employee.Department != "" && employee.Department != row.Name {
			return ErrInvalid
		}
		employee.Department = row.Name
	} else {
		err := tx.Clauses(shared).Take(&row, "name = ?", employee.Department).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			candidate := model.Department{Name: employee.Department, Version: 1}
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error
			if err == nil {
				err = tx.Clauses(shared).Take(&row, "name = ?", employee.Department).Error
			}
			if err == nil && candidate.ID == row.ID {
				err = writeDepartmentAudit(tx, actor, "CREATE", reason, nil, row)
			}
		}
		if err != nil {
			return err
		}
	}
	employee.DepartmentID = &row.ID
	return nil
}

func (s *Service) DepartmentTree(ctx context.Context, actor model.Actor) (model.DepartmentTree, error) {
	result := model.DepartmentTree{List: []model.DepartmentNode{}}
	if actor.AuthorityID != model.HRAdmin && actor.AuthorityID != model.HRViewer {
		return result, ErrForbidden
	}
	counts := s.visibleEmployees(ctx, actor).Select("department_id, COUNT(*) AS employee_count").
		Where("department_id IS NOT NULL").Group("department_id")
	var rows []struct {
		model.Department
		EmployeeCount int64
	}
	err := s.db.WithContext(ctx).Table("oa_departments AS d").
		Select("d.*, COALESCE(c.employee_count, 0) AS employee_count").
		Joins("LEFT JOIN (?) AS c ON c.department_id = d.id", counts).
		Order("d.id ASC").Scan(&rows).Error
	for _, row := range rows {
		result.List = append(result.List, model.DepartmentNode{
			DepartmentView: departmentView(row.Department), EmployeeCount: row.EmployeeCount,
		})
	}
	return result, err
}
