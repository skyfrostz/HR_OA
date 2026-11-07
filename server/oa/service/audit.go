package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

type auditFields map[string]any

func auditReason(reason, fallback string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = fallback
	}
	if !validText(reason, 500) {
		return "", ErrInvalid
	}
	return reason, nil
}

func writeAudit(tx *gorm.DB, actor model.Actor, event model.AuditEvent, before, after auditFields) error {
	event.ActorID = actor.UserID
	var err error
	event.Before, err = json.Marshal(before)
	if err != nil {
		return err
	}
	event.After, err = json.Marshal(after)
	if err != nil {
		return err
	}
	return tx.Create(&event).Error
}

func writeEmployeeAudit(tx *gorm.DB, actor model.Actor, action, reason string, before *model.Employee, after model.Employee) error {
	event := model.AuditEvent{EmployeeID: &after.ID, DepartmentID: after.DepartmentID, Action: action, Reason: reason}
	var oldSalary, oldDepartment auditFields
	if before != nil {
		oldSalary = auditFields{"salaryCents": before.SalaryCents}
		oldDepartment = auditFields{"departmentId": before.DepartmentID, "department": before.Department}
	}
	if before == nil || before.SalaryCents != after.SalaryCents {
		event.Type = model.AuditSalary
		if err := writeAudit(tx, actor, event, oldSalary, auditFields{"salaryCents": after.SalaryCents}); err != nil {
			return err
		}
	}
	if before == nil || before.Department != after.Department || !sameDepartment(before.DepartmentID, after.DepartmentID) {
		event.Type = model.AuditDepartment
		return writeAudit(tx, actor, event, oldDepartment, auditFields{"departmentId": after.DepartmentID, "department": after.Department})
	}
	return nil
}

func sameDepartment(a, b *uint) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func auditQuery(db, employees *gorm.DB, actor model.Actor, filter model.AuditFilter) *gorm.DB {
	query := db.Model(&model.AuditEvent{})
	if actor.AuthorityID == model.HRAdmin || actor.AuthorityID == model.HRViewer {
		query = query.Where("(employee_id IN (?) OR (employee_id IS NULL AND type = ?))", employees, model.AuditDepartment)
	} else {
		query = query.Where("employee_id IN (?)", employees)
	}
	if !actor.CanReadSalary() {
		query = query.Where("type <> ?", model.AuditSalary)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.EmployeeID != 0 {
		query = query.Where("employee_id = ?", filter.EmployeeID)
	}
	if filter.DepartmentID != 0 {
		query = query.Where("department_id = ?", filter.DepartmentID)
	}
	if filter.ContractID != 0 {
		query = query.Where("contract_id = ?", filter.ContractID)
	}
	return query
}

func (s *Service) AuditEvents(ctx context.Context, actor model.Actor, p model.Pagination, filter model.AuditFilter) (model.Page[model.AuditEvent], error) {
	page := model.NewPage[model.AuditEvent](p)
	switch filter.Type {
	case "", model.AuditSalary, model.AuditDepartment, model.AuditAnnualLeave, model.AuditContract, model.AuditAccount, model.AuditEmployment:
	default:
		return page, ErrInvalid
	}
	query := auditQuery(s.db.WithContext(ctx), s.visibleEmployees(ctx, actor).Select("oa_employees.id"), actor, filter)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	// A free-text reason can include compensation context even on a
	// simultaneous department change. Only HR administrators see that text.
	if actor.AuthorityID != model.HRAdmin {
		query = query.Omit("Reason")
	}
	err := query.Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&page.List).Error
	return page, err
}

func writeDepartmentAudit(tx *gorm.DB, actor model.Actor, action, reason string, before *model.Department, after model.Department) error {
	var old auditFields
	if before != nil {
		old = auditFields{"name": before.Name, "parentId": before.ParentID}
	}
	return writeAudit(tx, actor, model.AuditEvent{
		Type: model.AuditDepartment, DepartmentID: &after.ID, Action: action, Reason: reason,
	}, old, auditFields{"name": after.Name, "parentId": after.ParentID})
}

func leaveAuditFields(row model.AnnualLeave) auditFields {
	return auditFields{"year": row.Year, "entitledHalfDays": row.EntitledHalfDays, "usedHalfDays": row.UsedHalfDays}
}
