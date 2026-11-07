package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrReminderRateLimit = errors.New("reminder rate limited")

const reminderInterval = 5 * time.Minute

func (s *Service) ActOnRequest(ctx context.Context, actor model.Actor, id uint, key string, input model.RequestActionInput) (IntentResult, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	switch input.Action {
	case model.ActionCancel:
		if !actor.CanWrite() {
			return IntentResult{}, ErrForbidden
		}
	case model.ActionWithdraw, model.ActionRemind:
		if !actor.CanSubmitRequests() {
			return IntentResult{}, ErrForbidden
		}
	default:
		return IntentResult{}, ErrInvalid
	}
	if input.Reason == "" && input.Action == model.ActionRemind {
		input.Reason = "催办"
	}
	if !validText(input.Reason, 500) {
		return IntentResult{}, ErrInvalid
	}

	var person workflowUser
	return s.withIntent(ctx, actor.UserID, fmt.Sprintf("POST /api/oa/v1/requests/%d/actions", id), key, input, http.StatusOK,
		func(tx *gorm.DB) error {
			if err := checkWorkflowSession(tx, actor); err != nil {
				return err
			}
			if input.Action == model.ActionCancel {
				err := workflowUsers(tx).Where("u.id = ? AND u.authority_id = ?", actor.UserID, model.HRAdmin).Take(&person).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrForbidden
				}
				return err
			}
			var err error
			person, err = currentSubmitter(tx, actor.UserID)
			return err
		},
		func(tx *gorm.DB) (any, error) {
			return actOnRequest(tx, person, id, input)
		})
}

func actOnRequest(tx *gorm.DB, person workflowUser, id uint, input model.RequestActionInput) (model.RequestSummary, error) {
	var request model.ApprovalRequest
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	if input.Action == model.ActionCancel {
		query = query.Where("type = ? AND leave_calculation IS NOT NULL", "LEAVE")
	} else {
		query = query.Where("applicant_id = ?", person.ID)
	}
	if err := query.Take(&request, id).Error; err != nil {
		return model.RequestSummary{}, queryError(err)
	}
	now := workflowNow()
	event := model.ApprovalAction{
		RequestID: id, ActorID: person.ID, ActorName: person.DisplayName,
		Action: input.Action, Comment: input.Reason, CreatedAt: now,
	}
	if input.Action == model.ActionCancel {
		if request.Status != model.Approved {
			return model.RequestSummary{}, ErrInvalidState
		}
		if err := applyLeaveTransition(tx, request, model.LeaveReverse); err != nil {
			return model.RequestSummary{}, err
		}
		request.Status = model.Cancelled
	} else {
		if request.Status != model.RequestInReview || request.CurrentStep == nil {
			return model.RequestSummary{}, ErrInvalidState
		}
		var task model.ApprovalTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&task, "request_id = ? AND step = ? AND status = ?", request.ID, *request.CurrentStep, model.TaskPending).Error
		if err != nil {
			return model.RequestSummary{}, queryError(err)
		}
		event.TaskID = &task.ID
		if input.Action == model.ActionRemind {
			err := remindTask(tx, request, task, event, now)
			return requestSummary(request), err
		}
		if err := applyLeaveTransition(tx, request, model.LeaveRelease); err != nil {
			return model.RequestSummary{}, err
		}
		if err := tx.Model(&task).Updates(map[string]any{"status": model.Cancelled, "version": task.Version + 1, "completed_at": now}).Error; err != nil {
			return model.RequestSummary{}, err
		}
		request.Status = model.Withdrawn
	}
	request.CurrentStep, request.UpdatedAt = nil, now
	if err := tx.Model(&request).Updates(map[string]any{"status": request.Status, "current_step": nil, "updated_at": now}).Error; err != nil {
		return model.RequestSummary{}, err
	}
	summary := requestSummary(request)
	if input.Action == model.ActionCancel {
		summary.Title = ""
	}
	return summary, recordAction(tx, event, 0, "", now)
}

func remindTask(tx *gorm.DB, request model.ApprovalRequest, task model.ApprovalTask, event model.ApprovalAction, now time.Time) error {
	var count int64
	if err := tx.Model(&model.ApprovalAction{}).
		Where("request_id = ? AND action = ? AND created_at > ?", request.ID, model.ActionRemind, now.Add(-reminderInterval)).
		Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrReminderRateLimit
	}
	target, err := currentApprover(tx, task.AssigneeID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRouteUnavailable
	}
	if err != nil {
		return err
	}
	event.TargetUserID, event.TargetName = &target.ID, &target.DisplayName
	return recordAction(tx, event, target.ID, model.TaskReminder, now)
}

func approvedLeaveQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&model.ApprovalRequest{}).Where("type = ? AND status = ? AND leave_calculation IS NOT NULL", "LEAVE", model.Approved)
}

func (s *Service) ApprovedLeaves(ctx context.Context, actor model.Actor, p model.Pagination) (model.Page[model.ApprovedLeave], error) {
	page := model.NewPage[model.ApprovedLeave](p)
	if !actor.CanWrite() {
		return page, ErrForbidden
	}
	query := approvedLeaveQuery(s.db.WithContext(ctx))
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var rows []model.ApprovalRequest
	if err := query.Select("id", "applicant_id", "applicant_name", "leave_calculation", "updated_at").
		Order("id DESC").Limit(p.PageSize).Offset(p.Offset()).Find(&rows).Error; err != nil {
		return page, err
	}
	for _, row := range rows {
		calculation := row.LeaveCalculation
		if calculation == nil {
			return page, ErrInvalidState
		}
		page.List = append(page.List, model.ApprovedLeave{
			ID: row.ID, ApplicantID: row.ApplicantID, ApplicantName: row.ApplicantName, LeaveTypeName: calculation.LeaveTypeName,
			StartDate: calculation.StartDate, EndDate: calculation.EndDate, TotalHalfDays: calculation.TotalHalfDays, UpdatedAt: row.UpdatedAt.UTC(),
		})
	}
	return page, nil
}
