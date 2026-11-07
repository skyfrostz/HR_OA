package api

import (
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// LeaveTypes godoc
// @Tags OA Leave
// @Summary 查看可见请假类型
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.LeaveTypes}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-types [get]
func (a *API) LeaveTypes(c *gin.Context) {
	result, err := a.service.LeaveTypes(c.Request.Context(), Actor(c))
	sendRead(c, result, err)
}

// CreateLeaveType godoc
// @Tags OA Leave
// @Summary 创建请假类型
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param input body model.LeaveTypeInput true "提交字段"
// @Success 201 {object} response.Response{data=model.LeaveType}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-types [post]
func (a *API) CreateLeaveType(c *gin.Context) {
	var input model.LeaveTypeInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteLeaveType(c.Request.Context(), Actor(c), 0, input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}

// EditLeaveType godoc
// @Tags OA Leave
// @Summary 按版本更新请假类型
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.LeaveTypeInput true "提交字段"
// @Success 200 {object} response.Response{data=model.LeaveType}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-types/{id} [put]
func (a *API) EditLeaveType(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.LeaveTypeInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteLeaveType(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// LeaveCalendar godoc
// @Tags OA Leave
// @Summary 查看工作日历
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.LeaveCalendar}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-calendar [get]
func (a *API) LeaveCalendar(c *gin.Context) {
	result, err := a.service.LeaveCalendar(c.Request.Context(), Actor(c))
	sendRead(c, result, err)
}

// WriteLeaveCalendar godoc
// @Tags OA Leave
// @Summary 按版本更新工作日历
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param input body model.CalendarInput true "提交字段"
// @Success 200 {object} response.Response{data=model.LeaveCalendar}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-calendar [put]
func (a *API) WriteLeaveCalendar(c *gin.Context) {
	var input model.CalendarInput
	if !readBoundedJSON(c, &input, 64*1024) {
		return
	}
	result, err := a.service.WriteLeaveCalendar(c.Request.Context(), Actor(c), input)
	sendRead(c, result, err)
}

// PreviewLeave godoc
// @Tags OA Leave
// @Summary 计算工作半天、余额和重叠
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param input body model.LeavePeriod true "提交字段"
// @Success 200 {object} response.Response{data=model.LeavePreview}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/leave-preview [post]
func (a *API) PreviewLeave(c *gin.Context) {
	var input model.LeavePeriod
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.PreviewLeave(c.Request.Context(), Actor(c), input)
	sendRead(c, result, err)
}

// LeaveLedger godoc
// @Tags OA Leave
// @Summary 分页查看员工年度请假流水
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param year query int false "年度"
// @Success 200 {object} response.Response{data=model.Page[model.LeaveLedger]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/leave-ledger [get]
func (a *API) LeaveLedger(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	year := 0
	if !readPageNumber(c, "year", &year) || year > 9999 {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.LeaveLedger(c.Request.Context(), Actor(c), id, year, page)
	sendRead(c, result, err)
}

// ApprovalRules godoc
// @Tags OA Approvals
// @Summary 分页查看顺序审批规则
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.ApprovalRule]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/approval-rules [get]
func (a *API) ApprovalRules(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ApprovalRules(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// CreateApprovalRule godoc
// @Tags OA Approvals
// @Summary 创建顺序审批规则
// @Security ApiKeyAuth
// @Produce json
// @Accept json
// @Param input body model.ApprovalRuleInput true "提交字段"
// @Success 201 {object} response.Response{data=model.ApprovalRule}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/approval-rules [post]
func (a *API) CreateApprovalRule(c *gin.Context) {
	var input model.ApprovalRuleInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteApprovalRule(c.Request.Context(), Actor(c), 0, input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}

// EditApprovalRule godoc
// @Tags OA Approvals
// @Summary 按版本更新顺序审批规则
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.ApprovalRuleInput true "提交字段"
// @Success 200 {object} response.Response{data=model.ApprovalRule}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/approval-rules/{id} [put]
func (a *API) EditApprovalRule(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.ApprovalRuleInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteApprovalRule(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// RuleAssignees godoc
// @Tags OA Approvals
// @Summary 分页查看有效审批人
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.TransferTarget]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/approval-rules/assignees [get]
func (a *API) RuleAssignees(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.RuleAssignees(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// ActOnRequest godoc
// @Tags OA Approvals
// @Summary 幂等撤回、撤销已批准请假或催办
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.RequestActionInput true "提交字段"
// @Param Idempotency-Key header string true "意图唯一键"
// @Header 200 {string} Idempotency-Replayed "重放时为 true"
// @Success 200 {object} response.Response{data=model.RequestSummary}
// @Failure 400,401,403,404,409,413,422,429,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/requests/{id}/actions [post]
func (a *API) ActOnRequest(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	key, ok := intentKey(c)
	if !ok {
		return
	}
	var input model.RequestActionInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.ActOnRequest(c.Request.Context(), Actor(c), id, key, input)
	sendIntent(c, result, err)
}

// ApprovedLeaves godoc
// @Tags OA Leave
// @Summary 分页查看可撤销的新版已批请假
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.ApprovedLeave]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/approved-leaves [get]
func (a *API) ApprovedLeaves(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ApprovedLeaves(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// WorkflowExceptions godoc
// @Tags OA Approvals
// @Summary 分页查看不可用的审批步骤
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.WorkflowException]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/workflow-exceptions [get]
func (a *API) WorkflowExceptions(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.WorkflowExceptions(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// HandoffException godoc
// @Tags OA Approvals
// @Summary 按当前任务版本交接异常步骤
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.ExceptionHandoffInput true "提交字段"
// @Success 200 {object} response.Response{data=model.ExceptionHandoffResult}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/workflow-exceptions/{id}/handoff [post]
func (a *API) HandoffException(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.ExceptionHandoffInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.HandoffException(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// Dashboard godoc
// @Tags OA Dashboard
// @Summary 获取当前角色工作台计数
// @Security ApiKeyAuth
// @Produce json
// @Param days query int false "合同到期窗口天数"
// @Success 200 {object} response.Response{data=model.DashboardView}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/dashboard [get]
func (a *API) Dashboard(c *gin.Context) {
	days := 30
	if !readPageNumber(c, "days", &days) || days > 90 {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.Dashboard(c.Request.Context(), Actor(c), days)
	sendRead(c, result, err)
}
