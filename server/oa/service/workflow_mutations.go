package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const transferLimit = 20

func (s *Service) SubmitRequest(ctx context.Context, actor model.Actor, key string, input model.SubmitInput) (IntentResult, error) {
	if !actor.CanSubmitRequests() {
		return IntentResult{}, ErrForbidden
	}
	normalized, err := normalizeSubmission(input)
	if err != nil {
		return IntentResult{}, err
	}
	var applicant workflowUser
	return s.withIntent(ctx, actor.UserID, "POST /api/oa/v1/requests", key, normalized, http.StatusCreated,
		func(tx *gorm.DB) error {
			var err error
			applicant, err = currentSubmitter(tx, actor.UserID)
			return err
		},
		func(tx *gorm.DB) (model.RequestDetail, error) {
			return submitRequest(tx, applicant, normalized)
		},
	)
}

func submitRequest(tx *gorm.DB, applicant workflowUser, input model.SubmitInput) (model.RequestDetail, error) {
	if input.PreviousRequestID != nil {
		var prior model.ApprovalRequest
		err := tx.Select("id").Where("id = ? AND applicant_id = ? AND status = ?", *input.PreviousRequestID, applicant.ID, model.Rejected).
			Take(&prior).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.RequestDetail{}, ErrInvalid
		}
		if err != nil {
			return model.RequestDetail{}, err
		}
	}

	steps, err := developmentRoute(tx, applicant.ID, true)
	if err != nil {
		return model.RequestDetail{}, err
	}
	route, err := json.Marshal(steps)
	if err != nil {
		return model.RequestDetail{}, err
	}
	now := workflowNow()
	firstStep := steps[0].Step
	request := model.ApprovalRequest{
		Type: input.Type, Title: input.Title, Status: model.RequestInReview, ApplicantID: applicant.ID,
		ApplicantName: applicant.DisplayName, TemplateVersion: 1, Form: input.Form, Route: route,
		PreviousRequestID: input.PreviousRequestID, CurrentStep: &firstStep, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&request).Error; err != nil {
		return model.RequestDetail{}, err
	}
	if err := createPendingTask(tx, request.ID, steps[0], now); err != nil {
		return model.RequestDetail{}, err
	}
	audit := model.ApprovalAction{
		RequestID: request.ID, ActorID: applicant.ID, ActorName: applicant.DisplayName,
		Action: model.ActionSubmit, CreatedAt: now,
	}
	if err := recordAction(tx, audit, steps[0].AssigneeID, model.TaskAssigned, now); err != nil {
		return model.RequestDetail{}, err
	}
	return requestDetail(tx, request)
}

func (s *Service) ActOnTask(ctx context.Context, actor model.Actor, taskID uint, key string, input model.TaskActionInput) (IntentResult, error) {
	if !actor.CanApproveRequests() {
		return IntentResult{}, ErrForbidden
	}
	normalized, err := normalizeTaskAction(input)
	if err != nil {
		return IntentResult{}, err
	}
	var approver workflowUser
	return s.withIntent(ctx, actor.UserID, fmt.Sprintf("POST /api/oa/v1/tasks/%d/actions", taskID), key, normalized, http.StatusOK,
		func(tx *gorm.DB) error {
			var err error
			approver, err = currentApprover(tx, actor.UserID, false)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrForbidden
			}
			return err
		},
		func(tx *gorm.DB) (model.RequestDetail, error) {
			return actOnTask(tx, approver, taskID, normalized)
		},
	)
}

func actOnTask(tx *gorm.DB, approver workflowUser, taskID uint, input model.TaskActionInput) (model.RequestDetail, error) {
	var task model.ApprovalTask
	if err := tx.Select("request_id").Take(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.RequestDetail{}, ErrForbidden
		}
		return model.RequestDetail{}, err
	}
	// All transitions take the request lock before the task lock. The initial
	// non-locking lookup reads only the immutable task-to-request association.
	lock := clause.Locking{Strength: "UPDATE"}
	var request model.ApprovalRequest
	if err := tx.Clauses(lock).Take(&request, task.RequestID).Error; err != nil {
		return model.RequestDetail{}, err
	}
	if err := tx.Clauses(lock).Take(&task, taskID).Error; err != nil {
		return model.RequestDetail{}, err
	}
	if err := checkPendingTask(request, task, approver.ID, input.Version); err != nil {
		return model.RequestDetail{}, err
	}

	now := workflowNow()
	audit := model.ApprovalAction{
		RequestID: request.ID, TaskID: &task.ID, ActorID: approver.ID, ActorName: approver.DisplayName,
		Action: input.Action, Comment: input.Comment, CreatedAt: now,
	}
	taskUpdates := map[string]any{"version": task.Version + 1}
	recipient, kind, err := applyTaskAction(tx, &request, task, input, &audit, taskUpdates, now)
	if err != nil {
		return model.RequestDetail{}, err
	}
	updated := tx.Model(&model.ApprovalTask{}).
		Where("id = ? AND assignee_id = ? AND version = ? AND status = ?", task.ID, approver.ID, input.Version, model.TaskPending).
		Updates(taskUpdates)
	if updated.Error != nil {
		return model.RequestDetail{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		return model.RequestDetail{}, ErrStaleTask
	}
	request.UpdatedAt = now
	if err := tx.Model(&request).Updates(map[string]any{
		"status": request.Status, "current_step": request.CurrentStep, "updated_at": now,
	}).Error; err != nil {
		return model.RequestDetail{}, err
	}
	if err := recordAction(tx, audit, recipient, kind, now); err != nil {
		return model.RequestDetail{}, err
	}
	return requestDetail(tx, request)
}

func checkPendingTask(request model.ApprovalRequest, task model.ApprovalTask, userID uint, version int) error {
	if task.AssigneeID != userID || request.ApplicantID == userID {
		return ErrForbidden
	}
	if task.Status != model.TaskPending || request.Status != model.RequestInReview ||
		request.CurrentStep == nil || *request.CurrentStep != task.Step || (version != 0 && task.Version != version) {
		return ErrStaleTask
	}
	return nil
}

func applyTaskAction(tx *gorm.DB, request *model.ApprovalRequest, task model.ApprovalTask, input model.TaskActionInput,
	audit *model.ApprovalAction, taskUpdates map[string]any, now time.Time,
) (uint, string, error) {
	switch input.Action {
	case model.ActionTransfer:
		target, err := transferTask(tx, *request, task, *input.TargetUserID)
		if err != nil {
			return 0, "", err
		}
		audit.TargetUserID, audit.TargetName = &target.ID, &target.DisplayName
		taskUpdates["assignee_id"], taskUpdates["assignee_name"] = target.ID, target.DisplayName
		return target.ID, model.TaskAssigned, nil
	case model.ActionReject:
		taskUpdates["status"], taskUpdates["completed_at"] = model.Rejected, now
		request.Status, request.CurrentStep = model.Rejected, nil
		return request.ApplicantID, model.RequestRejected, nil
	case model.ActionApprove:
		next, err := nextRouteStep(*request, task.Step)
		if err != nil {
			return 0, "", err
		}
		taskUpdates["status"], taskUpdates["completed_at"] = model.Approved, now
		if next == nil {
			request.Status, request.CurrentStep = model.Approved, nil
			return request.ApplicantID, model.RequestApproved, nil
		}
		if _, err := currentApprover(tx, next.AssigneeID, false); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = ErrRouteUnavailable
			}
			return 0, "", err
		}
		if err := createPendingTask(tx, request.ID, *next, now); err != nil {
			return 0, "", err
		}
		request.CurrentStep = &next.Step
		return next.AssigneeID, model.TaskAssigned, nil
	default:
		return 0, "", ErrInvalid
	}
}

func nextRouteStep(request model.ApprovalRequest, current int) (*model.RouteStep, error) {
	var steps []model.RouteStep
	if err := json.Unmarshal(request.Route, &steps); err != nil {
		return nil, err
	}
	if current < 1 || current > len(steps) {
		return nil, errors.New("invalid saved workflow step")
	}
	if current == len(steps) {
		return nil, nil
	}
	next := steps[current]
	if next.Step != current+1 || next.AssigneeID == 0 || next.AssigneeID == request.ApplicantID {
		return nil, ErrRouteUnavailable
	}
	return &next, nil
}

func transferTask(tx *gorm.DB, request model.ApprovalRequest, task model.ApprovalTask, targetID uint) (workflowUser, error) {
	if targetID == task.AssigneeID || targetID == request.ApplicantID {
		return workflowUser{}, ErrInvalid
	}
	var count int64
	if err := tx.Model(&model.ApprovalAction{}).Where("request_id = ? AND action = ?", request.ID, model.ActionTransfer).
		Count(&count).Error; err != nil {
		return workflowUser{}, err
	}
	if count >= transferLimit {
		return workflowUser{}, ErrTransferLimit
	}
	target, err := currentApprover(tx, targetID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrInvalid
	}
	return target, err
}

func createPendingTask(tx *gorm.DB, requestID uint, step model.RouteStep, now time.Time) error {
	return tx.Create(&model.ApprovalTask{
		RequestID: requestID, Step: step.Step, StepName: step.Name, AssigneeID: step.AssigneeID,
		AssigneeName: step.AssigneeName, Status: model.TaskPending, Version: 1, CreatedAt: now,
	}).Error
}

func recordAction(tx *gorm.DB, action model.ApprovalAction, recipient uint, kind string, now time.Time) error {
	if err := tx.Create(&action).Error; err != nil {
		return err
	}
	return tx.Create(&model.NotificationEvent{
		ActionID: action.ID, RequestID: action.RequestID, RecipientID: recipient,
		Kind: kind, AvailableAt: now, CreatedAt: now,
	}).Error
}

func workflowNow() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
