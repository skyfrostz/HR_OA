package service

import (
	"encoding/json"
	"errors"

	system "github.com/flipped-aurora/gin-vue-admin/server/model/system"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockWorkflow(tx *gorm.DB, strength string) error {
	var guard model.WorkflowGuard
	return tx.Clauses(clause.Locking{Strength: strength}).Take(&guard, 1).Error
}

func checkWorkflowSession(tx *gorm.DB, actor model.Actor) error {
	if err := lockWorkflow(tx, "SHARE"); err != nil {
		return err
	}
	var state model.AccountSession
	err := tx.Take(&state, "user_id = ?", actor.UserID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if state.Epoch != actor.SessionEpoch {
		return ErrUnauthorized
	}
	return nil
}

type handoffMap map[uint]uint

func loadHandoffs(db *gorm.DB) (handoffMap, error) {
	var rows []model.ApprovalHandoff
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(handoffMap, len(rows))
	for _, row := range rows {
		result[row.UserID] = row.TargetUserID
	}
	return result, nil
}

func (h handoffMap) resolve(id uint) (uint, error) {
	for hops := 0; h[id] != 0; hops++ {
		if hops >= len(h) {
			return 0, ErrHandoffConflict
		}
		id = h[id]
	}
	return id, nil
}

func resolveRouteAssignee(db *gorm.DB, handoffs handoffMap, source, applicant uint) (workflowUser, error) {
	id, err := handoffs.resolve(source)
	if err != nil || id == applicant || id == 0 {
		return workflowUser{}, ErrRouteUnavailable
	}
	person, err := currentApprover(db, id, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrRouteUnavailable
	}
	return person, err
}

func handoffApprovals(tx *gorm.DB, actor model.Actor, user system.SysUser, targetID *uint) error {
	handoffs, err := loadHandoffs(tx)
	if err != nil {
		return err
	}
	var pending []model.ApprovalTask
	err = tx.Table("oa_tasks AS t").Select("t.*").
		Joins("JOIN oa_requests AS r ON r.id = t.request_id").
		Where("t.assignee_id = ? AND t.status = ? AND r.status = ? AND r.current_step = t.step",
			user.ID, model.TaskPending, model.RequestInReview).Order("t.id ASC").Scan(&pending).Error
	if err != nil {
		return err
	}
	future, needed, err := futureHandoffs(tx, handoffs, user.ID, targetID)
	if err != nil {
		return err
	}
	if !needed && len(pending) == 0 {
		if targetID != nil {
			return ErrInvalid
		}
		return nil
	}
	if targetID == nil {
		return ErrHandoffRequired
	}
	// A replacement must be eligible itself, rather than resolving to a
	// different user. Re-enabled historical sources keep their outgoing edge.
	if *targetID == user.ID || handoffs[*targetID] != 0 {
		return ErrHandoffConflict
	}
	target, err := currentApprover(tx, *targetID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrHandoffConflict
	}
	if err != nil {
		return err
	}
	if needed {
		if err := tx.Create(&model.ApprovalHandoff{UserID: user.ID, TargetUserID: target.ID}).Error; err != nil {
			return err
		}
	}
	now := workflowNow()
	if len(future) != 0 {
		events := make([]model.ApprovalAction, 0, len(future))
		for _, id := range future {
			events = append(events, model.ApprovalAction{
				RequestID: id, ActorID: actor.UserID, ActorName: "人事管理员",
				Action: model.ActionHandoff, Comment: "人事安排后续审批交接",
				TargetUserID: &target.ID, TargetName: &target.DisplayName, CreatedAt: now,
			})
		}
		// A future assignee receives access and notification only when a task
		// is assigned. This entry makes the change visible to participants.
		if err := tx.CreateInBatches(&events, 100).Error; err != nil {
			return err
		}
	}
	for _, task := range pending {
		var request model.ApprovalRequest
		if err := tx.Select("id", "applicant_id").Take(&request, task.RequestID).Error; err != nil {
			return err
		}
		if request.ApplicantID == target.ID {
			return ErrHandoffConflict
		}
		result := tx.Model(&model.ApprovalTask{}).
			Where("id = ? AND version = ? AND status = ? AND assignee_id = ?", task.ID, task.Version, model.TaskPending, user.ID).
			Updates(map[string]any{"assignee_id": target.ID, "assignee_name": target.DisplayName, "version": task.Version + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStaleTask
		}
		if err := tx.Model(&model.ApprovalRequest{}).Where("id = ?", request.ID).Update("updated_at", now).Error; err != nil {
			return err
		}
		event := model.ApprovalAction{
			RequestID: task.RequestID, TaskID: &task.ID, ActorID: actor.UserID,
			ActorName: "人事管理员", Action: model.ActionHandoff, Comment: "人事审批交接",
			PreviousAssigneeID: &user.ID, TargetUserID: &target.ID, TargetName: &target.DisplayName, CreatedAt: now,
		}
		if err := recordAction(tx, event, target.ID, model.TaskAssigned, now); err != nil {
			return err
		}
	}
	return nil
}

func futureHandoffs(tx *gorm.DB, handoffs handoffMap, source uint, target *uint) ([]uint, bool, error) {
	var defaults []system.SysUser
	if err := tx.Select("id").Where("username IN ?", []string{"manager", "finance"}).Find(&defaults).Error; err != nil {
		return nil, false, err
	}
	needed := false
	for _, user := range defaults {
		terminal, err := handoffs.resolve(user.ID)
		if err != nil {
			return nil, false, err
		}
		needed = needed || terminal == source
	}
	var future []uint
	var requests []model.ApprovalRequest
	err := tx.Select("id", "applicant_id", "current_step", "route").
		Where("status = ?", model.RequestInReview).
		FindInBatches(&requests, 100, func(_ *gorm.DB, _ int) error {
			for _, request := range requests {
				affected, err := futureHandoff(request, handoffs, source, target)
				if err != nil {
					return err
				}
				if affected {
					future = append(future, request.ID)
				}
			}
			return nil
		}).Error
	return future, needed || len(future) != 0, err
}

func futureHandoff(request model.ApprovalRequest, handoffs handoffMap, source uint, target *uint) (bool, error) {
	var steps []model.RouteStep
	if err := json.Unmarshal(request.Route, &steps); err != nil {
		return false, err
	}
	if request.CurrentStep == nil {
		return false, ErrHandoffConflict
	}
	for _, step := range steps {
		if step.Step <= *request.CurrentStep {
			continue
		}
		terminal, err := handoffs.resolve(step.AssigneeID)
		if err != nil {
			return false, err
		}
		if terminal == source {
			if target != nil && *target == request.ApplicantID {
				return false, ErrHandoffConflict
			}
			return true, nil
		}
	}
	return false, nil
}
