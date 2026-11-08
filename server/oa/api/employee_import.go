package api

import (
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// EmployeeImportTemplate godoc
// @Tags OA Employee Workbooks
// @Summary 下载空白员工导入模板
// @Security ApiKeyAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success 200 {file} file
// @Failure 401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/import-template.xlsx [get]
func (a *API) EmployeeImportTemplate(c *gin.Context) {
	data, err := a.service.EmployeeImportTemplate(Actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	sendWorkbook(c, "employee-import-template.xlsx", data)
}

// PreviewEmployeeImport godoc
// @Tags OA Employee Workbooks
// @Summary 验证员工工作簿并创建持久预览
// @Security ApiKeyAuth
// @Accept mpfd
// @Produce json
// @Param file formData file true "员工导入 XLSX，最多 2 MiB"
// @Success 201 {object} response.Response{data=model.ImportPreview}
// @Failure 400,401,403,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/preview [post]
func (a *API) PreviewEmployeeImport(c *gin.Context) {
	header, cleanup, err := singleFileUpload(c, service.MaxEmployeeWorkbookSize)
	defer cleanup()
	if err != nil {
		fail(c, err)
		return
	}
	if !strings.EqualFold(path.Ext(header.Filename), ".xlsx") {
		fail(c, service.ErrInvalid)
		return
	}
	file, err := header.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer file.Close()
	result, err := a.service.PreviewEmployeeImport(c.Request.Context(), Actor(c), file)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}

// EmployeeImportPreview godoc
// @Tags OA Employee Workbooks
// @Summary 查看本人创建的导入预览
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "不透明预览 ID"
// @Param errorsOnly query bool false "仅返回有错误的源行"
// @Success 200 {object} response.Response{data=model.ImportPreview}
// @Failure 401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/{id} [get]
func (a *API) EmployeeImportPreview(c *gin.Context) {
	result, err := a.service.EmployeeImportPreview(c.Request.Context(), Actor(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if values, present := c.GetQueryArray("errorsOnly"); present {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			fail(c, service.ErrInvalid)
			return
		}
		if values[0] == "true" {
			rows := make([]model.ImportRow, 0, result.ErrorRows)
			for _, row := range result.Rows {
				if len(row.Errors) != 0 {
					rows = append(rows, row)
				}
			}
			result.Rows = rows
		}
	}
	succeed(c, http.StatusOK, result)
}

type importConfirmInput struct{}

// DiscardEmployeeImport godoc
// @Tags OA Employee Workbooks
// @Summary 将本人未确认的预览设为失效并清理源行
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path string true "不透明预览 ID"
// @Param discard body importConfirmInput true "空对象"
// @Success 200 {object} response.Response{data=model.ImportPreview}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/{id}/discard [post]
func (a *API) DiscardEmployeeImport(c *gin.Context) {
	var input *importConfirmInput
	if !readJSON(c, &input) {
		return
	}
	if input == nil {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.DiscardEmployeeImport(c.Request.Context(), Actor(c), c.Param("id"))
	sendRead(c, result, err)
}

// ConfirmEmployeeImport godoc
// @Tags OA Employee Workbooks
// @Summary 按持久预览 ID 幂等确认整批导入
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path string true "不透明预览 ID"
// @Param confirmation body importConfirmInput true "空对象"
// @Success 200 {object} response.Response{data=model.ImportResult}
// @Failure 400,401,403,404,409,410,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employee-imports/{id}/confirm [post]
func (a *API) ConfirmEmployeeImport(c *gin.Context) {
	var input *importConfirmInput
	if !readJSON(c, &input) {
		return
	}
	if input == nil {
		fail(c, service.ErrInvalid)
		return
	}
	result, err := a.service.ConfirmEmployeeImport(c.Request.Context(), Actor(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// ExportEmployees godoc
// @Tags OA Employee Workbooks
// @Summary 导出当前权限范围内的员工记录
// @Security ApiKeyAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success 200 {file} file
// @Failure 401,403,409,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/export.xlsx [get]
func (a *API) ExportEmployees(c *gin.Context) {
	data, err := a.service.ExportEmployees(c.Request.Context(), Actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	sendWorkbook(c, "employees.xlsx", data)
}

func sendWorkbook(c *gin.Context, name string, data []byte) {
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Data(http.StatusOK, xlsxType, data)
}
