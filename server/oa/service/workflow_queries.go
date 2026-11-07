package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
)

func myRequestsQuery(db *gorm.DB, actor model.Actor) *gorm.DB {
	return db.Model(&model.ApprovalRequest{}).Where("applicant_id = ?", actor.UserID)
}

func (s *Service) MyRequests(ctx context.Context, actor model.Actor, p model.Pagination, filter model.WorkflowFilter) (model.Page[model.RequestSummary], error) {
	page := model.NewPage[model.RequestSummary](p)
	query := myRequestsQuery(s.db.WithContext(ctx), actor)
	query, err := applyWorkflowFilter(query, filter, "", "created_at")
	if err != nil {
		return page, err
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var requests []model.ApprovalRequest
	if err := query.Omit("Form", "Route", "LeaveCalculation", "RuleSnapshot", "RouteOverrides").Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&requests).Error; err != nil {
		return page, err
	}
	for _, request := range requests {
		page.List = append(page.List, requestSummary(request))
	}
	return page, nil
}

func (s *Service) Request(ctx context.Context, actor model.Actor, id uint) (model.RequestDetail, error) {
	var detail model.RequestDetail
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request model.ApprovalRequest
		err := tx.Where(`id = ? AND (
			applicant_id = ?
			OR EXISTS (SELECT 1 FROM oa_tasks AS t WHERE t.request_id = oa_requests.id AND t.assignee_id = ?)
			OR EXISTS (SELECT 1 FROM oa_request_actions AS a WHERE a.request_id = oa_requests.id AND ((a.actor_id = ? AND a.action = ?) OR (a.previous_assignee_id = ? AND a.action = ? AND a.task_id IS NOT NULL)))
		)`, id, actor.UserID, actor.UserID, actor.UserID, model.ActionTransfer, actor.UserID, model.ActionHandoff).Take(&request).Error
		if err != nil {
			return queryError(err)
		}
		detail, err = requestDetail(tx, request)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return detail, err
}

func pendingTasksQuery(db *gorm.DB, actor model.Actor) *gorm.DB {
	query := db.Table("oa_tasks AS t").Joins("JOIN oa_requests AS r ON r.id = t.request_id").
		Where("t.assignee_id = ? AND t.status = ? AND r.status = ? AND r.current_step = t.step",
			actor.UserID, model.TaskPending, model.RequestInReview)
	if !actor.CanApproveRequests() || actor.EmployeeID == nil {
		query = query.Where("1 = 0")
	}
	return query
}

func (s *Service) PendingTasks(ctx context.Context, actor model.Actor, p model.Pagination, filter model.WorkflowFilter) (model.Page[model.TaskView], error) {
	page := model.NewPage[model.TaskView](p)
	query := pendingTasksQuery(s.db.WithContext(ctx), actor)
	query, err := applyWorkflowFilter(query, filter, "r.", "t.created_at")
	if err != nil {
		return page, err
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var records []struct {
		model.ApprovalTask
		RequestTitle  string
		RequestType   string
		ApplicantName string
	}
	err = query.Select("t.*, r.title AS request_title, r.type AS request_type, r.applicant_name").
		Order("t.id DESC").Limit(p.PageSize).Offset(p.Offset()).Scan(&records).Error
	if err != nil {
		return page, err
	}
	for _, record := range records {
		request := model.ApprovalRequest{Title: record.RequestTitle, Type: record.RequestType, ApplicantName: record.ApplicantName}
		page.List = append(page.List, taskView(record.ApprovalTask, request))
	}
	return page, nil
}

func (s *Service) TaskHistory(ctx context.Context, actor model.Actor, p model.Pagination, filter model.WorkflowFilter) (model.Page[model.ActionView], error) {
	page := model.NewPage[model.ActionView](p)
	query := s.db.WithContext(ctx).Table("oa_request_actions AS a").Joins("JOIN oa_requests AS r ON r.id = a.request_id").
		Where("a.actor_id = ? AND a.action IN ?", actor.UserID, []string{model.ActionApprove, model.ActionReject, model.ActionTransfer})
	query, err := applyWorkflowFilter(query, filter, "r.", "a.created_at")
	if err != nil {
		return page, err
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var records []struct {
		model.ApprovalAction
		RequestTitle string
		RequestType  string
	}
	err = query.Select("a.*, r.title AS request_title, r.type AS request_type").
		Order("a.id DESC").Limit(p.PageSize).Offset(p.Offset()).Scan(&records).Error
	if err != nil {
		return page, err
	}
	for _, record := range records {
		request := model.ApprovalRequest{Title: record.RequestTitle, Type: record.RequestType}
		page.List = append(page.List, actionView(record.ApprovalAction, request))
	}
	return page, nil
}

func (s *Service) TransferTargets(ctx context.Context, actor model.Actor, taskID uint) (model.TransferTargetsView, error) {
	result := model.TransferTargetsView{List: []model.TransferTarget{}}
	if !actor.CanApproveRequests() {
		return result, ErrForbidden
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task model.ApprovalTask
		if err := tx.Take(&task, taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrForbidden
			}
			return err
		}
		var request model.ApprovalRequest
		if err := tx.Select("applicant_id", "status", "current_step").Take(&request, task.RequestID).Error; err != nil {
			return err
		}
		if err := checkPendingTask(request, task, actor.UserID, 0); err != nil {
			return err
		}
		return boundWorkflowUsers(tx).
			Select("u.id, u.nick_name AS display_name").
			Where("u.authority_id = ? AND u.id NOT IN ?", model.ApproverRole, []uint{actor.UserID, request.ApplicantID}).
			Order("u.id ASC").Find(&result.List).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func (s *Service) Notifications(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.NotificationView], error) {
	page := model.NewPage[model.NotificationView](p)
	query := s.db.WithContext(ctx).Model(&model.MockNotification{}).Where("recipient_id = ?", actor.UserID)
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var records []model.MockNotification
	if err := query.Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&records).Error; err != nil {
		return page, err
	}
	for _, record := range records {
		page.List = append(page.List, model.NotificationView{
			ID: record.ID, EventID: record.EventID, RequestID: record.RequestID,
			Kind: record.Kind, Title: record.Title, CreatedAt: record.CreatedAt.UTC(), DeliveryMode: "mock",
		})
	}
	return page, nil
}

func requestSummary(request model.ApprovalRequest) model.RequestSummary {
	return model.RequestSummary{
		ID: request.ID, Type: request.Type, Title: request.Title, Status: request.Status, ApplicantID: request.ApplicantID,
		ApplicantName: request.ApplicantName, CreatedAt: request.CreatedAt.UTC(), UpdatedAt: request.UpdatedAt.UTC(),
		PreviousRequestID: request.PreviousRequestID, CurrentStep: request.CurrentStep,
	}
}

func requestDetail(tx *gorm.DB, request model.ApprovalRequest) (model.RequestDetail, error) {
	detail := model.RequestDetail{
		RequestSummary: requestSummary(request), TemplateVersion: request.TemplateVersion, Form: request.Form,
		LeaveCalculation: request.LeaveCalculation,
		Tasks:            []model.TaskView{}, History: []model.ActionView{},
	}
	if request.RuleSnapshot != nil {
		rule := request.RuleSnapshot
		detail.Rule = &model.RuleReference{ID: rule.ID, Version: rule.Version, Name: rule.Name}
	}
	if err := json.Unmarshal(request.Route, &detail.Route); err != nil {
		return detail, err
	}
	var tasks []model.ApprovalTask
	if err := tx.Where("request_id = ?", request.ID).Order("step ASC").Find(&tasks).Error; err != nil {
		return detail, err
	}
	for _, task := range tasks {
		detail.Tasks = append(detail.Tasks, taskView(task, request))
	}
	var actions []model.ApprovalAction
	if err := tx.Where("request_id = ?", request.ID).Order("id ASC").Find(&actions).Error; err != nil {
		return detail, err
	}
	for _, action := range actions {
		detail.History = append(detail.History, actionView(action, request))
	}
	return detail, nil
}

func taskView(task model.ApprovalTask, request model.ApprovalRequest) model.TaskView {
	return model.TaskView{
		ID: task.ID, RequestID: task.RequestID, RequestTitle: request.Title, RequestType: request.Type,
		ApplicantName: request.ApplicantName, Step: task.Step, StepName: task.StepName, AssigneeID: task.AssigneeID,
		AssigneeName: task.AssigneeName, Status: task.Status, Version: task.Version,
		CreatedAt: task.CreatedAt.UTC(), CompletedAt: task.CompletedAt,
	}
}

func actionView(action model.ApprovalAction, request model.ApprovalRequest) model.ActionView {
	return model.ActionView{
		ID: action.ID, RequestID: action.RequestID, RequestTitle: request.Title, RequestType: request.Type,
		TaskID: action.TaskID, ActorID: action.ActorID, ActorName: action.ActorName,
		RouteStep: action.RouteStep, PreviousAssigneeID: action.PreviousAssigneeID, Action: action.Action,
		Comment: action.Comment, TargetUserID: action.TargetUserID, TargetName: action.TargetName,
		CreatedAt: action.CreatedAt.UTC(),
	}
}
