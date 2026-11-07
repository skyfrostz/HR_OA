package service

import (
	"context"
	"errors"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

var ErrRuleConflict = errors.New("overlapping approval rules")

func validRequestType(kind string) bool {
	return kind == "LEAVE" || kind == "EXPENSE" || kind == "GENERAL"
}

func ruleFromInput(input model.ApprovalRuleInput) (model.ApprovalRule, error) {
	rule := model.ApprovalRule{Name: strings.TrimSpace(input.Name), Type: input.Type, Priority: input.Priority,
		DepartmentID: input.DepartmentID, MinAmountCents: input.MinAmountCents, MaxAmountCents: input.MaxAmountCents,
		Steps: make([]model.ApprovalRuleStep, 0, len(input.Steps)), Version: input.Version + 1}
	if !validText(rule.Name, 100) || !validRequestType(rule.Type) || input.Enabled == nil ||
		rule.Priority < 1 || rule.Priority > 10000 || len(input.Steps) < 1 || len(input.Steps) > 10 ||
		(input.DepartmentID != nil && *input.DepartmentID == 0) {
		return rule, ErrInvalid
	}
	rule.Enabled = *input.Enabled
	if rule.Type != "EXPENSE" && (rule.MinAmountCents != nil || rule.MaxAmountCents != nil) {
		return rule, ErrInvalid
	}
	for _, bound := range []*int64{rule.MinAmountCents, rule.MaxAmountCents} {
		if bound != nil && (*bound < 1 || *bound > maxSalaryCents) {
			return rule, ErrInvalid
		}
	}
	if rule.MinAmountCents != nil && rule.MaxAmountCents != nil && *rule.MinAmountCents > *rule.MaxAmountCents {
		return rule, ErrInvalid
	}
	for _, step := range input.Steps {
		step.Name = strings.TrimSpace(step.Name)
		if step.AssigneeID == 0 || !validText(step.Name, 100) {
			return rule, ErrInvalid
		}
		rule.Steps = append(rule.Steps, step)
	}
	return rule, nil
}

func rulesOverlap(a, b model.ApprovalRule) bool {
	if a.DepartmentID != nil && b.DepartmentID != nil && *a.DepartmentID != *b.DepartmentID {
		return false
	}
	return !(a.MaxAmountCents != nil && b.MinAmountCents != nil && *a.MaxAmountCents < *b.MinAmountCents ||
		b.MaxAmountCents != nil && a.MinAmountCents != nil && *b.MaxAmountCents < *a.MinAmountCents)
}

func (s *Service) ApprovalRules(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.ApprovalRule], error) {
	page := model.NewPage[model.ApprovalRule](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	query := s.db.WithContext(ctx).Model(&model.ApprovalRule{})
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := query.Order("priority DESC, id ASC").Limit(p.PageSize).Offset(p.Offset()).Find(&page.List).Error
	return page, err
}

func (s *Service) RuleAssignees(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.TransferTarget], error) {
	page := model.NewPage[model.TransferTarget](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	query := boundWorkflowUsers(s.db.WithContext(ctx)).Where("u.authority_id = ?", model.ApproverRole).
		Where("NOT EXISTS (SELECT 1 FROM oa_approval_handoffs AS h WHERE h.user_id = u.id)")
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := query.Select("u.id, u.nick_name AS display_name").Order("u.id ASC").Limit(p.PageSize).Offset(p.Offset()).Scan(&page.List).Error
	return page, err
}

func (s *Service) WriteApprovalRule(ctx context.Context, actor model.Actor, id uint, input model.ApprovalRuleInput) (model.ApprovalRule, error) {
	if !actor.CanWrite() {
		return model.ApprovalRule{}, ErrForbidden
	}
	rule, err := ruleFromInput(input)
	if err != nil || (id == 0 && input.Version != 0) || (id != 0 && input.Version < 1) {
		return rule, ErrInvalid
	}
	rule.ID = id
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		event := model.AuditEvent{Type: auditApprovalRule, Action: "CREATE", Reason: "设置审批规则", DepartmentID: rule.DepartmentID}
		var before auditFields
		if id != 0 {
			var old model.ApprovalRule
			if err := tx.Take(&old, id).Error; err != nil {
				return queryError(err)
			}
			if old.Version != input.Version {
				return ErrStaleRecord
			}
			event.Action, before = "UPDATE", ruleFields(old)
		}
		if rule.DepartmentID != nil {
			var department model.Department
			if err := tx.Select("id").Take(&department, *rule.DepartmentID).Error; err != nil {
				return queryError(err)
			}
		}
		if rule.Enabled {
			if _, err := ruleRoute(tx, rule, 0); err != nil {
				return err
			}
			var competing []model.ApprovalRule
			if err := tx.Where("id <> ? AND enabled = ? AND type = ? AND priority = ?", id, true, rule.Type, rule.Priority).
				Find(&competing).Error; err != nil {
				return err
			}
			for _, other := range competing {
				if rulesOverlap(rule, other) {
					return ErrRuleConflict
				}
			}
		}
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, event, before, ruleFields(rule))
	})
	return rule, recordWriteError(err)
}

func ruleFields(rule model.ApprovalRule) auditFields {
	return auditFields{"id": rule.ID, "version": rule.Version, "name": rule.Name, "type": rule.Type, "enabled": rule.Enabled,
		"departmentId": rule.DepartmentID, "minAmountCents": rule.MinAmountCents, "maxAmountCents": rule.MaxAmountCents,
		"priority": rule.Priority, "steps": rule.Steps}
}

func ruleRoute(db *gorm.DB, rule model.ApprovalRule, applicantID uint) ([]model.RouteStep, error) {
	handoffs, err := loadHandoffs(db)
	if err != nil {
		return nil, err
	}
	steps := make([]model.RouteStep, 0, len(rule.Steps))
	for index, step := range rule.Steps {
		person, err := resolveRouteAssignee(db, handoffs, step.AssigneeID, applicantID)
		if err != nil {
			return nil, err
		}
		steps = append(steps, model.RouteStep{Step: index + 1, Name: step.Name, AssigneeID: person.ID, AssigneeName: person.DisplayName})
	}
	if len(steps) == 0 {
		return nil, ErrRouteUnavailable
	}
	return steps, nil
}

func selectApprovalRoute(db *gorm.DB, kind string, departmentID *uint, amount *int64, applicantID uint) ([]model.RouteStep, *model.ApprovalRule, error) {
	query := db.Where("enabled = ? AND type = ?", true, kind)
	if departmentID == nil {
		query = query.Where("department_id IS NULL")
	} else {
		query = query.Where("(department_id IS NULL OR department_id = ?)", *departmentID)
	}
	if amount == nil {
		query = query.Where("min_amount_cents IS NULL AND max_amount_cents IS NULL")
	} else {
		query = query.Where("(min_amount_cents IS NULL OR min_amount_cents <= ?) AND (max_amount_cents IS NULL OR max_amount_cents >= ?)", *amount, *amount)
	}
	var rule model.ApprovalRule
	err := query.Order("priority DESC, id ASC").Take(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		route, err := developmentRoute(db, applicantID)
		return route, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	route, err := ruleRoute(db, rule, applicantID)
	return route, &rule, err
}
