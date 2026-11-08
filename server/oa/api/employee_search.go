package api

import (
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// SearchEmployees godoc
// @Tags OA Employees
// @Summary 在授权范围内按姓名、工号、部门字面文本及部门ID查询员工
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param filter body model.EmployeeSearch true "query最多100字符；departmentScope默认DIRECT，SUBTREE要求departmentId；分页默认1/20"
// @Success 200 {object} response.Response{data=model.Page[model.EmployeeView]}
// @Failure 400,401,403,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/search [post]
func (a *API) SearchEmployees(c *gin.Context) {
	var input *model.EmployeeSearch
	if !readJSON(c, &input) {
		return
	}
	if input == nil {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.SearchEmployees(c.Request.Context(), Actor(c), *input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// ExportFilteredEmployees godoc
// @Tags OA Employee Workbooks
// @Summary 导出授权范围内符合筛选条件的员工（最多5000条）
// @Security ApiKeyAuth
// @Accept json
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param filter body model.EmployeeFilter true "与员工查询相同的筛选，不接受分页参数"
// @Success 200 {file} file
// @Failure 400,401,403,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/export.xlsx [post]
func (a *API) ExportFilteredEmployees(c *gin.Context) {
	var input *model.EmployeeFilter
	if !readJSON(c, &input) {
		return
	}
	if input == nil {
		fail(c, service.ErrInvalid)
		return
	}
	data, err := a.service.ExportFilteredEmployees(c.Request.Context(), Actor(c), *input)
	if err != nil {
		fail(c, err)
		return
	}
	sendWorkbook(c, "employees.xlsx", data)
}
