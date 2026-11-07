package api

import (
	"net/http"
	"strconv"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// AuditEvents godoc
// @Tags OA Audit
// @Summary 按权限查看业务变更历史
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param type query string false "事件类型"
// @Param employeeId query int false "员工 ID"
// @Param departmentId query int false "部门 ID"
// @Param contractId query int false "合同 ID"
// @Success 200 {object} response.Response{data=model.Page[model.AuditEvent]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/audit-events [get]
func (a *API) AuditEvents(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	filter := model.AuditFilter{Type: c.Query("type")}
	for name, id := range map[string]*uint{
		"employeeId": &filter.EmployeeID, "departmentId": &filter.DepartmentID, "contractId": &filter.ContractID,
	} {
		values, present := c.GetQueryArray(name)
		if !present {
			continue
		}
		if len(values) != 1 {
			fail(c, service.ErrInvalid)
			return
		}
		parsed, err := strconv.ParseUint(values[0], 10, strconv.IntSize)
		if err != nil || parsed == 0 {
			fail(c, service.ErrInvalid)
			return
		}
		*id = uint(parsed)
	}
	result, err := a.service.AuditEvents(c.Request.Context(), Actor(c), page, filter)
	sendRead(c, result, err)
}

func sendRead(c *gin.Context, data any, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, data)
}

// EmployeeAccount godoc
// @Tags OA Accounts
// @Summary 查看员工账号绑定
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Success 200 {object} response.Response{data=model.EmployeeAccount}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/account [get]
func (a *API) EmployeeAccount(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.EmployeeAccount(c.Request.Context(), Actor(c), id)
	sendRead(c, result, err)
}

// AccountCandidates godoc
// @Tags OA Accounts
// @Summary 分页查看未绑定的可管理账号
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.AccountDetails]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/account-candidates [get]
func (a *API) AccountCandidates(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.AccountCandidates(c.Request.Context(), Actor(c), id, page)
	sendRead(c, result, err)
}

// AccountHandoffTargets godoc
// @Tags OA Accounts
// @Summary 分页查看可交接审批人
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} response.Response{data=model.Page[model.TransferTarget]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/handoff-targets [get]
func (a *API) AccountHandoffTargets(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.AccountHandoffTargets(c.Request.Context(), Actor(c), id, page)
	sendRead(c, result, err)
}

// ActOnAccount godoc
// @Tags OA Accounts
// @Summary 按版本操作员工账号及在职状态
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.AccountAction true "操作字段"
// @Success 200 {object} response.Response{data=model.EmployeeAccount}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/account/actions [post]
func (a *API) ActOnAccount(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.AccountAction
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.ActOnAccount(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// Contract godoc
// @Tags OA Contracts
// @Summary 获取指定合同及其附件
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Success 200 {object} response.Response{data=model.ContractView}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/contracts/{id} [get]
func (a *API) Contract(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.Contract(c.Request.Context(), Actor(c), id)
	sendRead(c, result, err)
}

// EditContract godoc
// @Tags OA Contracts
// @Summary 按版本编辑草稿合同
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.ContractEdit true "操作字段"
// @Success 200 {object} response.Response{data=model.ContractView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/contracts/{id} [put]
func (a *API) EditContract(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.ContractEdit
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.EditContract(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// ActOnContract godoc
// @Tags OA Contracts
// @Summary 按版本登记签署或终止合同
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "记录 ID"
// @Accept json
// @Param input body model.ContractAction true "操作字段"
// @Success 200 {object} response.Response{data=model.ContractView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/contracts/{id}/actions [post]
func (a *API) ActOnContract(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.ContractAction
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.ActOnContract(c.Request.Context(), Actor(c), id, input)
	sendRead(c, result, err)
}

// DepartmentTree godoc
// @Tags OA Personnel
// @Summary 获取部门树节点和直接员工人数
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.DepartmentTree}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/departments/tree [get]
func (a *API) DepartmentTree(c *gin.Context) {
	result, err := a.service.DepartmentTree(c.Request.Context(), Actor(c))
	sendRead(c, result, err)
}
