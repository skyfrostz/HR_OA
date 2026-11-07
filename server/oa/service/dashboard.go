package service

import (
	"context"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
)

func (s *Service) Dashboard(ctx context.Context, actor model.Actor, days int) (model.DashboardView, error) {
	var result model.DashboardView
	db := s.db.WithContext(ctx)
	if err := myRequestsQuery(db, actor).Count(&result.MyRequests).Error; err != nil {
		return result, err
	}
	if err := pendingTasksQuery(db, actor).Count(&result.PendingTasks).Error; err != nil {
		return result, err
	}
	if err := s.contractReminderQuery(ctx, actor, days).Count(&result.ContractReminders).Error; err != nil {
		return result, err
	}
	if actor.CanWrite() {
		exceptions, err := s.WorkflowExceptions(ctx, actor, model.Pagination{Page: 1, PageSize: 1})
		if err != nil {
			return result, err
		}
		result.WorkflowExceptions = &exceptions.Total
		var approved int64
		if err := approvedLeaveQuery(db).Count(&approved).Error; err != nil {
			return result, err
		}
		result.ApprovedLeaves = &approved
	}
	return result, nil
}
