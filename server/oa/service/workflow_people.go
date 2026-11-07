package service

import (
	"context"
	"errors"

	system "github.com/flipped-aurora/gin-vue-admin/server/model/system"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type workflowUser struct {
	ID          uint
	Username    string
	DisplayName string
	AuthorityID uint
}

// A live workflow identity has an enabled account and active GVA authority.
// Submission and transfer candidates also require a current employee binding.
func workflowUsers(db *gorm.DB) *gorm.DB {
	return db.Table("sys_users AS u").
		Select("u.id, u.username, u.nick_name AS display_name, u.authority_id").
		Joins("JOIN sys_authorities AS a ON a.authority_id = u.authority_id AND a.deleted_at IS NULL").
		Where("u.deleted_at IS NULL AND u.enable = ?", 1).
		Where("NOT EXISTS (SELECT 1 FROM oa_employees AS bound WHERE bound.user_id = u.id AND bound.deleted_at IS NULL AND bound.employment_status = ?)", model.EmploymentOffboarded)
}

func boundWorkflowUsers(db *gorm.DB) *gorm.DB {
	return workflowUsers(db).Joins("JOIN oa_employees AS e ON e.user_id = u.id AND e.deleted_at IS NULL AND e.employment_status = ?", model.EmploymentActive)
}

func currentSubmitter(tx *gorm.DB, id uint) (workflowUser, error) {
	var person workflowUser
	err := boundWorkflowUsers(tx).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("u.id = ? AND u.authority_id IN ?", id, []uint{model.EmployeeRole, model.ApproverRole}).
		Take(&person).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrForbidden
	}
	return person, err
}

func currentApprover(db *gorm.DB, id uint, requireBinding bool) (workflowUser, error) {
	var person workflowUser
	query := workflowUsers(db)
	if requireBinding {
		query = boundWorkflowUsers(db)
	}
	err := query.Clauses(clause.Locking{Strength: "SHARE"}).
		Where("u.id = ? AND u.authority_id = ?", id, model.ApproverRole).Take(&person).Error
	return person, err
}

func developmentRoute(db *gorm.DB, applicantID uint) ([]model.RouteStep, error) {
	var defaults []system.SysUser
	if err := db.Select("id", "username").Where("username IN ?", []string{"manager", "finance"}).Find(&defaults).Error; err != nil {
		return nil, err
	}
	if len(defaults) != 2 {
		return nil, ErrRouteUnavailable
	}
	byName := make(map[string]uint, 2)
	for _, user := range defaults {
		byName[user.Username] = user.ID
	}
	handoffs, err := loadHandoffs(db)
	if err != nil {
		return nil, err
	}
	manager, err := resolveRouteAssignee(db, handoffs, byName["manager"], applicantID)
	if err != nil {
		return nil, err
	}
	finance, err := resolveRouteAssignee(db, handoffs, byName["finance"], applicantID)
	if err != nil {
		return nil, err
	}
	return []model.RouteStep{
		{Step: 1, Name: "负责人审批", AssigneeID: manager.ID, AssigneeName: manager.DisplayName},
		{Step: 2, Name: "复核审批", AssigneeID: finance.ID, AssigneeName: finance.DisplayName},
	}, nil
}

func (s *Service) RequestTemplates(ctx context.Context, actor model.Actor) (model.TemplatesView, error) {
	if !actor.CanSubmitRequests() && !actor.CanApproveRequests() {
		return model.TemplatesView{}, ErrForbidden
	}
	// Approvers on the demonstration route can browse templates. A submission
	// still rejects a route that would include the applicant.
	steps, err := developmentRoute(s.db.WithContext(ctx), 0)
	if err != nil {
		return model.TemplatesView{}, err
	}
	return model.TemplatesView{
		IntegrationMode: "mock",
		List: []model.RequestTemplate{
			{Type: "LEAVE", Name: "请假申请", Version: 1, Description: "填写请假日期和原因", Route: steps},
			{Type: "EXPENSE", Name: "报销申请", Version: 1, Description: "填写费用日期、金额和原因", Route: steps},
			{Type: "GENERAL", Name: "通用申请", Version: 1, Description: "填写申请事项", Route: steps},
		},
	}, nil
}
