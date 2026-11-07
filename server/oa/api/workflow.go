package api

import (
	"encoding/json"
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// RequestTemplates godoc
// @Tags OA Approvals
// @Summary 获取模拟开发审批模板
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.TemplatesView}
// @Failure 401,403,409,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/request-templates [get]
func (a *API) RequestTemplates(c *gin.Context) {
	result, err := a.service.RequestTemplates(c.Request.Context(), Actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// SubmitRequest godoc
// @Tags OA Approvals
// @Summary 幂等提交新申请或驳回重提
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "每次申请意图的唯一键"
// @Param request body model.SubmitInput true "申请与表单"
// @Success 201 {object} response.Response{data=model.RequestDetail}
// @Header 201 {string} Idempotency-Replayed "重放时为 true"
// @Failure 400,401,403,409,413,422,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/requests [post]
func (a *API) SubmitRequest(c *gin.Context) {
	key, ok := intentKey(c)
	if !ok {
		return
	}
	var input model.SubmitInput
	// A legal 2,000-character form can use escaped surrogate pairs in JSON.
	if !readBoundedJSON(c, &input, 32*1024) {
		return
	}
	result, err := a.service.SubmitRequest(c.Request.Context(), Actor(c), key, input)
	sendIntent(c, result, err)
}

// MyRequests godoc
// @Tags OA Approvals
// @Summary 分页获取本人申请
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Param type query string false "申请类型"
// @Param status query string false "申请状态"
// @Param fromDate query string false "起始业务日期 YYYY-MM-DD"
// @Param toDate query string false "结束业务日期 YYYY-MM-DD"
// @Success 200 {object} response.Response{data=model.Page[model.RequestSummary]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/requests [get]
func (a *API) MyRequests(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	filter, ok := workflowFilter(c)
	if !ok {
		return
	}
	result, err := a.service.MyRequests(c.Request.Context(), Actor(c), page, filter)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// RequestDetail godoc
// @Tags OA Approvals
// @Summary 获取本人申请或实际分配过的申请详情
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "申请ID"
// @Success 200 {object} response.Response{data=model.RequestDetail}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/requests/{id} [get]
func (a *API) RequestDetail(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.Request(c.Request.Context(), Actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// PendingTasks godoc
// @Tags OA Approvals
// @Summary 分页获取本人待办
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Param type query string false "申请类型"
// @Param status query string false "申请状态"
// @Param fromDate query string false "起始业务日期 YYYY-MM-DD"
// @Param toDate query string false "结束业务日期 YYYY-MM-DD"
// @Success 200 {object} response.Response{data=model.Page[model.TaskView]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/tasks [get]
func (a *API) PendingTasks(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	filter, ok := workflowFilter(c)
	if !ok {
		return
	}
	result, err := a.service.PendingTasks(c.Request.Context(), Actor(c), page, filter)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// TaskHistory godoc
// @Tags OA Approvals
// @Summary 分页获取本人实际作出的决定与转交
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Param type query string false "申请类型"
// @Param status query string false "申请状态"
// @Param fromDate query string false "起始业务日期 YYYY-MM-DD"
// @Param toDate query string false "结束业务日期 YYYY-MM-DD"
// @Success 200 {object} response.Response{data=model.Page[model.ActionView]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/tasks/history [get]
func (a *API) TaskHistory(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	filter, ok := workflowFilter(c)
	if !ok {
		return
	}
	result, err := a.service.TaskHistory(c.Request.Context(), Actor(c), page, filter)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// TransferTargets godoc
// @Tags OA Approvals
// @Summary 获取当前待办可转交对象
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} response.Response{data=model.TransferTargetsView}
// @Failure 400,401,403,409,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/tasks/{id}/transfer-targets [get]
func (a *API) TransferTargets(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.TransferTargets(c.Request.Context(), Actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// ActOnTask godoc
// @Tags OA Approvals
// @Summary 幂等通过、驳回或转交当前任务
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "任务ID"
// @Param Idempotency-Key header string true "每次审批动作意图的唯一键"
// @Param action body model.TaskActionInput true "审批动作、任务版本与备注"
// @Success 200 {object} response.Response{data=model.RequestDetail}
// @Header 200 {string} Idempotency-Replayed "重放时为 true"
// @Failure 400,401,403,409,413,422,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/tasks/{id}/actions [post]
func (a *API) ActOnTask(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	key, ok := intentKey(c)
	if !ok {
		return
	}
	var payload struct {
		model.TaskActionInput
		Target json.RawMessage `json:"targetUserId"`
	}
	if !readJSON(c, &payload) {
		return
	}
	if len(payload.Target) != 0 {
		if payload.Action != model.ActionTransfer || json.Unmarshal(payload.Target, &payload.TargetUserID) != nil || payload.TargetUserID == nil {
			fail(c, service.ErrInvalid)
			return
		}
	}
	result, err := a.service.ActOnTask(c.Request.Context(), Actor(c), id, key, payload.TaskActionInput)
	sendIntent(c, result, err)
}

// Notifications godoc
// @Tags OA Approvals
// @Summary 分页获取发送给本人的模拟通知
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.NotificationView]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/notifications [get]
func (a *API) Notifications(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.Notifications(c.Request.Context(), Actor(c), page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

func intentKey(c *gin.Context) (string, bool) {
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		fail(c, service.ErrInvalid)
		return "", false
	}
	return keys[0], true
}

func sendIntent(c *gin.Context, result service.IntentResult, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	succeed(c, result.Status, result.Body)
}

func workflowFilter(c *gin.Context) (model.WorkflowFilter, bool) {
	var filter model.WorkflowFilter
	for name, target := range map[string]*string{
		"type": &filter.Type, "status": &filter.Status,
		"fromDate": &filter.FromDate, "toDate": &filter.ToDate,
	} {
		if values, ok := c.GetQueryArray(name); ok {
			if len(values) != 1 {
				fail(c, service.ErrInvalid)
				return filter, false
			}
			*target = values[0]
		}
	}
	return filter, true
}
