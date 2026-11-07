package api

import (
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/gin-gonic/gin"
)

// ListEmployees godoc
// @Tags OA Employees
// @Summary 分页获取可见员工
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.EmployeeView]}
// @Failure 400,401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees [get]
func (a *API) ListEmployees(c *gin.Context) {
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ListEmployees(c.Request.Context(), Actor(c), page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// Employee godoc
// @Tags OA Employees
// @Summary 获取可见员工详情
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "员工ID"
// @Success 200 {object} response.Response{data=model.EmployeeView}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id} [get]
func (a *API) Employee(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	result, err := a.service.Employee(c.Request.Context(), Actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// CreateEmployee godoc
// @Tags OA Employees
// @Summary HR 管理员新建员工
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param employee body model.EmployeeInput true "员工资料"
// @Success 201 {object} response.Response{data=model.EmployeeView}
// @Failure 400,401,403,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees [post]
func (a *API) CreateEmployee(c *gin.Context) {
	var input model.EmployeeInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.CreateEmployee(c.Request.Context(), Actor(c), input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}
