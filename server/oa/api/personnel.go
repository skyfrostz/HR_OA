package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// Departments godoc
// @Tags OA Personnel
// @Summary 分页列出部门及层级关系
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.DepartmentView]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/departments [get]
func (a *API) Departments(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.Departments(c.Request.Context(), Actor(c), page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// CreateDepartment godoc
// @Tags OA Personnel
// @Summary 创建部门
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param department body model.DepartmentInput true "部门名称与上级"
// @Success 201 {object} response.Response{data=model.DepartmentView}
// @Failure 400,401,403,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/departments [post]
func (a *API) CreateDepartment(c *gin.Context) {
	var input model.DepartmentInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteDepartment(c.Request.Context(), Actor(c), 0, model.DepartmentEdit{DepartmentInput: input})
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}

// EditDepartment godoc
// @Tags OA Personnel
// @Summary 更新部门名称与层级
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "部门 ID"
// @Param department body model.DepartmentEdit true "部门名称、上级与当前版本"
// @Success 200 {object} response.Response{data=model.DepartmentView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/departments/{id} [put]
func (a *API) EditDepartment(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var payload struct {
		model.DepartmentEdit
		Parent json.RawMessage `json:"parentId"`
	}
	if !readJSON(c, &payload) {
		return
	}
	if len(payload.Parent) == 0 || json.Unmarshal(payload.Parent, &payload.ParentID) != nil {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.WriteDepartment(c.Request.Context(), Actor(c), id, payload.DepartmentEdit)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// EditEmployee godoc
// @Tags OA Personnel
// @Summary 按当前版本更新员工档案
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "员工 ID"
// @Param employee body model.EmployeeEdit true "完整员工档案与当前版本"
// @Success 200 {object} response.Response{data=model.EmployeeView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id} [put]
func (a *API) EditEmployee(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.EmployeeEdit
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.EditEmployee(c.Request.Context(), Actor(c), id, input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// AnnualLeave godoc
// @Tags OA Personnel
// @Summary 分页查看员工的手动年假记录
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "员工 ID"
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.AnnualLeaveView]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/annual-leave [get]
func (a *API) AnnualLeave(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.AnnualLeave(c.Request.Context(), Actor(c), id, page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// WriteAnnualLeave godoc
// @Tags OA Personnel
// @Summary 新建或按版本更新手动年假记录
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "员工 ID"
// @Param year path int true "年度" minimum(2000) maximum(2100)
// @Param record body model.AnnualLeaveInput true "半日额度、已用量、备注和当前版本"
// @Success 200 {object} response.Response{data=model.AnnualLeaveView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/annual-leave/{year} [put]
func (a *API) WriteAnnualLeave(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	year, err := strconv.Atoi(c.Param("year"))
	if err != nil {
		fail(c, service.ErrInvalid)
		return
	}
	var input model.AnnualLeaveInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.WriteAnnualLeave(c.Request.Context(), Actor(c), id, year, input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// ContractReminders godoc
// @Tags OA Contracts
// @Summary 查看权限范围内的即将到期和逾期合同
// @Security ApiKeyAuth
// @Produce json
// @Param days query int false "到期窗口" default(30) minimum(1) maximum(90)
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.ContractReminder]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/contract-reminders [get]
func (a *API) ContractReminders(c *gin.Context) {
	days := 30
	if !readPageNumber(c, "days", &days) || days > 90 {
		fail(c, service.ErrInvalid)
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ContractReminders(c.Request.Context(), Actor(c), days, page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}
