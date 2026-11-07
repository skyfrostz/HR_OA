package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// IntegrationStatus godoc
// @Tags OA Integration
// @Summary 获取当前集成模式
// @Produce json
// @Success 200 {object} response.Response{data=model.IntegrationStatus}
// @Router /api/oa/v1/integration/status [get]
func (a *API) IntegrationStatus(c *gin.Context) {
	succeed(c, http.StatusOK, a.service.IntegrationStatus())
}

func wecomCookie(c *gin.Context, origin, kind, value string, age int) {
	secure := strings.HasPrefix(origin, "https://")
	name := "oa-wecom-" + kind
	if secure {
		name = "__Host-" + name
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, MaxAge: age, Path: "/",
		Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func readWeComCookie(c *gin.Context, origin, kind string) string {
	name := "oa-wecom-" + kind
	if strings.HasPrefix(origin, "https://") {
		name = "__Host-" + name
	}
	value, _ := c.Cookie(name)
	return value
}

// StartWeCom godoc
// @Tags OA Auth
// @Summary 开始一次性企业微信授权
// @Param entry query string true "pc 或 h5"
// @Success 302
// @Failure 400,429,503 {object} response.Response{data=errorData}
// @Router /api/oa/v1/auth/wecom/start [get]
func (a *API) StartWeCom(c *gin.Context) {
	entries := c.QueryArray("entry")
	if len(entries) != 1 {
		fail(c, service.ErrInvalid)
		return
	}
	location, state, err := a.service.StartWeCom(c.Request.Context(), entries[0])
	if err != nil {
		fail(c, err)
		return
	}
	origin := a.service.WeComOrigin()
	wecomCookie(c, origin, "ticket", "", -1)
	wecomCookie(c, origin, "state", state, 300)
	c.Redirect(http.StatusFound, location)
}

// CompleteWeCom godoc
// @Tags OA Auth
// @Summary 处理企业微信授权回调
// @Param state query string true "一次性状态"
// @Param code query string true "企业微信授权码"
// @Success 303
// @Failure 429,503 {object} response.Response{data=errorData}
// @Router /api/oa/v1/auth/wecom/callback [get]
func (a *API) CompleteWeCom(c *gin.Context) {
	origin := a.service.WeComOrigin()
	if origin == "" {
		fail(c, service.ErrIntegrationDisabled)
		return
	}
	var state, code string
	if values := c.QueryArray("state"); len(values) == 1 {
		state = values[0]
	}
	if values := c.QueryArray("code"); len(values) == 1 {
		code = values[0]
	}
	entry, ticket, err := a.service.CompleteWeCom(c.Request.Context(), state, readWeComCookie(c, origin, "state"), code)
	wecomCookie(c, origin, "state", "", -1)
	result := "failed"
	if err == nil {
		wecomCookie(c, origin, "ticket", ticket, 90)
		result = "ready"
	} else {
		wecomCookie(c, origin, "ticket", "", -1)
	}
	page := "/oa.html"
	if entry == "h5" {
		page = "/h5.html"
	}
	c.Redirect(http.StatusSeeOther, origin+page+"?wecom="+result)
}

type wecomExchangeInput struct{}

// ExchangeWeCom godoc
// @Tags OA Auth
// @Summary 用一次性同源票据建立 OA 会话
// @Accept json
// @Produce json
// @Param input body wecomExchangeInput true "空对象"
// @Success 200 {object} response.Response{data=model.LoginView}
// @Failure 400,401,429,503 {object} response.Response{data=errorData}
// @Router /api/oa/v1/auth/wecom/exchange [post]
func (a *API) ExchangeWeCom(c *gin.Context) {
	origin := a.service.WeComOrigin()
	if origin == "" {
		fail(c, service.ErrIntegrationDisabled)
		return
	}
	if requestOrigin := c.GetHeader("Origin"); requestOrigin != "" && requestOrigin != origin {
		fail(c, service.ErrUnauthorized)
		return
	}
	var input *wecomExchangeInput
	if !readJSON(c, &input) {
		return
	}
	if input == nil {
		fail(c, service.ErrInvalid)
		return
	}
	value := readWeComCookie(c, origin, "ticket")
	wecomCookie(c, origin, "ticket", "", -1)
	result, err := a.service.ExchangeWeCom(c.Request.Context(), value)
	sendRead(c, result, err)
}

// WeComBinding godoc
// @Tags OA Integration
// @Summary 查看员工企业微信绑定
// @Security ApiKeyAuth
// @Param id path int true "员工 ID"
// @Success 200 {object} response.Response{data=model.WeComBindingView}
// @Failure 401,403,404 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/wecom-binding [get]
func (a *API) WeComBinding(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.WeComBinding(c.Request.Context(), Actor(c), id)
	sendRead(c, result, err)
}

// WriteWeComBinding godoc
// @Tags OA Integration
// @Summary 绑定或解绑已有员工账号
// @Security ApiKeyAuth
// @Accept json
// @Param id path int true "员工 ID"
// @Param input body model.WeComBindingInput true "绑定变更"
// @Success 200 {object} response.Response{data=model.WeComBindingView}
// @Failure 400,401,403,404,409,503 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/wecom-binding [put]
func (a *API) WriteWeComBinding(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input struct {
		model.WeComBindingInput
		RawMember json.RawMessage `json:"memberId"`
	}
	if !readJSON(c, &input) {
		return
	}
	if len(input.RawMember) == 0 || json.Unmarshal(input.RawMember, &input.MemberID) != nil {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.WriteWeComBinding(c.Request.Context(), Actor(c), id, input.WeComBindingInput)
	sendRead(c, result, err)
}

// NotificationDeliveries godoc
// @Tags OA Integration
// @Summary 查看通知处理状态
// @Security ApiKeyAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.NotificationDeliveryView]}
// @Failure 400,401,403 {object} response.Response{data=errorData}
// @Router /api/oa/v1/notification-deliveries [get]
func (a *API) NotificationDeliveries(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.NotificationDeliveries(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// UpdateClassification godoc
// @Tags OA Contracts
// @Summary 按版本更新附件访问分级
// @Security ApiKeyAuth
// @Accept json
// @Param id path int true "附件 ID"
// @Param input body model.AttachmentClassification true "分级变更"
// @Success 200 {object} response.Response{data=model.FileView}
// @Failure 400,401,403,404,409 {object} response.Response{data=errorData}
// @Router /api/oa/v1/files/{id}/classification [put]
func (a *API) UpdateClassification(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.AttachmentClassification
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.UpdateClassification(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// ImportHistory godoc
// @Tags OA Employee Workbooks
// @Summary 查看本人导入历史摘要
// @Security ApiKeyAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.ImportHistoryItem]}
// @Failure 400,401,403 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports [get]
func (a *API) ImportHistory(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ImportHistory(c.Request.Context(), Actor(c), page)
	sendRead(c, result, err)
}

// EmployeeImportErrors godoc
// @Tags OA Employee Workbooks
// @Summary 导出未过期预览中的错误行
// @Security ApiKeyAuth
// @Param id path string true "预览 ID"
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success 200 {file} file
// @Failure 401,403,404,409,410 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/{id}/errors.xlsx [get]
func (a *API) EmployeeImportErrors(c *gin.Context) {
	data, err := a.service.EmployeeImportErrors(c.Request.Context(), Actor(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	sendWorkbook(c, "employee-import-errors.xlsx", data)
}

// EmployeeImportResult godoc
// @Tags OA Employee Workbooks
// @Summary 读取本人已确认导入的结果
// @Security ApiKeyAuth
// @Param id path string true "预览 ID"
// @Success 200 {object} response.Response{data=model.ImportResult}
// @Failure 401,403,404,409 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/{id}/result [get]
func (a *API) EmployeeImportResult(c *gin.Context) {
	result, err := a.service.EmployeeImportResult(c.Request.Context(), Actor(c), c.Param("id"))
	sendRead(c, result, err)
}
