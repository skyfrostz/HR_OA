package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	currentUnavailable = "CURRENT_UNAVAILABLE"
	futureUnavailable  = "FUTURE_UNAVAILABLE"
)

func effectiveRouteStep(request model.ApprovalRequest, original model.RouteStep) model.RouteStep {
	if replacement, ok := request.RouteOverrides[original.Step]; ok {
		return replacement
	}
	return original
}

func approvalAvailability(db *gorm.DB) (map[uint]bool, handoffMap, error) {
	var ids []uint
	if err := boundWorkflowUsers(db).Select("u.id").Where("u.authority_id = ?", model.ApproverRole).Pluck("u.id", &ids).Error; err != nil {
		return nil, nil, err
	}
	available := make(map[uint]bool, len(ids))
	for _, id := range ids {
		available[id] = true
	}
	handoffs, err := loadHandoffs(db)
	return available, handoffs, err
}

func exceptionFor(request model.ApprovalRequest, task model.ApprovalTask, available map[uint]bool, handoffs handoffMap) (*model.WorkflowException, error) {
	result := model.WorkflowException{TaskID: task.ID, RequestID: request.ID, RequestType: request.Type,
		Version: task.Version, CreatedAt: task.CreatedAt.UTC()}
	if !available[task.AssigneeID] || task.AssigneeID == request.ApplicantID {
		result.Step, result.StepName, result.AssigneeID, result.Reason = task.Step, task.StepName, task.AssigneeID, currentUnavailable
		return &result, nil
	}
	var route []model.RouteStep
	if err := json.Unmarshal(request.Route, &route); err != nil {
		return nil, err
	}
	for _, original := range route {
		if original.Step <= task.Step {
			continue
		}
		step := effectiveRouteStep(request, original)
		id, err := handoffs.resolve(step.AssigneeID)
		if err != nil {
			id = step.AssigneeID
		}
		if err != nil || !available[id] || id == request.ApplicantID {
			result.Step, result.StepName, result.AssigneeID, result.Reason = step.Step, step.Name, id, futureUnavailable
			return &result, nil
		}
	}
	return nil, nil
}

func (s *Service) WorkflowExceptions(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.WorkflowException], error) {
	page := model.NewPage[model.WorkflowException](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		available, handoffs, err := approvalAvailability(tx)
		if err != nil {
			return err
		}
		rows, err := tx.Table("oa_tasks AS t").Joins("JOIN oa_requests AS r ON r.id = t.request_id").
			Where("t.status = ? AND r.status = ? AND r.current_step = t.step", model.TaskPending, model.RequestInReview).
			Select("t.id, t.step, t.step_name, t.assignee_id, t.version, t.created_at, r.id, r.type, r.applicant_id, r.route, r.route_overrides").
			Order("t.id DESC").Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task model.ApprovalTask
			var request model.ApprovalRequest
			var route, overrides []byte
			if err := rows.Scan(&task.ID, &task.Step, &task.StepName, &task.AssigneeID, &task.Version, &task.CreatedAt,
				&request.ID, &request.Type, &request.ApplicantID, &route, &overrides); err != nil {
				return err
			}
			request.Route = route
			if len(overrides) != 0 {
				if err := json.Unmarshal(overrides, &request.RouteOverrides); err != nil {
					return err
				}
			}
			exception, err := exceptionFor(request, task, available, handoffs)
			if err != nil {
				return err
			}
			if exception == nil {
				continue
			}
			if page.Total >= int64(p.Offset()) && len(page.List) < p.PageSize {
				page.List = append(page.List, *exception)
			}
			page.Total++
		}
		return rows.Err()
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return page, err
}

func (s *Service) HandoffException(ctx context.Context, actor model.Actor, taskID uint, input model.ExceptionHandoffInput) (model.ExceptionHandoffResult, error) {
	var result model.ExceptionHandoffResult
	if !actor.CanWrite() {
		return result, ErrForbidden
	}
	reason, err := auditReason(input.Reason, "")
	if err != nil || input.Version < 1 || input.TargetUserID == 0 || input.Step < 1 {
		return result, ErrInvalid
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx = s.notificationTx(tx)
		if err := lockWorkflow(tx, "UPDATE"); err != nil {
			return err
		}
		var task model.ApprovalTask
		if err := tx.Select("request_id").Take(&task, taskID).Error; err != nil {
			return queryError(err)
		}
		lock := clause.Locking{Strength: "UPDATE"}
		var request model.ApprovalRequest
		if err := tx.Clauses(lock).Take(&request, task.RequestID).Error; err != nil {
			return queryError(err)
		}
		if err := tx.Clauses(lock).Take(&task, taskID).Error; err != nil {
			return queryError(err)
		}
		if task.Version != input.Version || task.Status != model.TaskPending || request.Status != model.RequestInReview ||
			request.CurrentStep == nil || *request.CurrentStep != task.Step {
			return ErrStaleTask
		}
		available, handoffs, err := approvalAvailability(tx)
		if err != nil {
			return err
		}
		exception, err := exceptionFor(request, task, available, handoffs)
		if err != nil {
			return err
		}
		if exception == nil {
			return ErrInvalidState
		}
		if input.Step != exception.Step {
			return ErrStaleTask
		}
		if input.TargetUserID == request.ApplicantID || !available[input.TargetUserID] || handoffs[input.TargetUserID] != 0 {
			return ErrHandoffConflict
		}
		target, err := currentApprover(tx, input.TargetUserID, true)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHandoffConflict
		}
		if err != nil {
			return err
		}
		now := workflowNow()
		event := model.ApprovalAction{
			RequestID: request.ID, ActorID: actor.UserID, ActorName: "人事管理员",
			Action: model.ActionHandoff, Comment: reason, CreatedAt: now, TargetUserID: &target.ID, TargetName: &target.DisplayName,
			RouteStep: &exception.Step, PreviousAssigneeID: &exception.AssigneeID,
		}
		taskUpdates := map[string]any{"version": task.Version + 1}
		notification := ""
		if exception.Reason == currentUnavailable {
			event.TaskID = &task.ID
			taskUpdates["assignee_id"], taskUpdates["assignee_name"] = target.ID, target.DisplayName
			notification = model.TaskAssigned
		} else {
			if request.RouteOverrides == nil {
				request.RouteOverrides = make(map[int]model.RouteStep)
			}
			request.RouteOverrides[exception.Step] = model.RouteStep{Step: exception.Step, Name: exception.StepName,
				AssigneeID: target.ID, AssigneeName: target.DisplayName}
		}
		if err := tx.Model(&model.ApprovalTask{}).Where("id = ?", task.ID).Updates(taskUpdates).Error; err != nil {
			return err
		}
		request.UpdatedAt = now
		if err := tx.Model(&request).Select("RouteOverrides", "UpdatedAt").Updates(&request).Error; err != nil {
			return err
		}
		if err := recordAction(tx, event, target.ID, notification, now); err != nil {
			return err
		}
		result = model.ExceptionHandoffResult{TaskID: task.ID, RequestID: request.ID, Version: task.Version + 1, AssigneeID: target.ID}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return result, recordWriteError(err)
}
