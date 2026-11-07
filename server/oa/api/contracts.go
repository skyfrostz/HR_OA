package api

import (
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/gin-gonic/gin"
)

// ListContracts godoc
// @Tags OA Contracts
// @Summary 分页获取员工的合同历史和附件元数据
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "员工ID"
// @Param page query int false "页码" default(1) minimum(1)
// @Param pageSize query int false "每页条数" default(20) minimum(1) maximum(100)
// @Success 200 {object} response.Response{data=model.Page[model.ContractView]}
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/contracts [get]
func (a *API) ListContracts(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	page, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := a.service.ListContracts(c.Request.Context(), Actor(c), id, page)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

// CreateContract godoc
// @Tags OA Contracts
// @Summary HR 管理员创建合同或关联原合同续约
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "员工ID"
// @Param contract body model.ContractInput true "合同资料"
// @Success 201 {object} response.Response{data=model.ContractView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/employees/{id}/contracts [post]
func (a *API) CreateContract(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var input model.ContractInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.CreateContract(c.Request.Context(), Actor(c), id, input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}
