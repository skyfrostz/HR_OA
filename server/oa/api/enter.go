package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"

	"github.com/flipped-aurora/gin-vue-admin/server/model/common/response"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

const maxJSONSize = 16 * 1024

type API struct {
	service *service.Service
}

func New(svc *service.Service) *API { return &API{service: svc} }

type errorData struct {
	ErrorCode string `json:"errorCode"`
}

func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, response.Response{
		Code: response.ERROR, Data: errorData{ErrorCode: code}, Msg: message,
	})
}

func succeed(c *gin.Context, status int, data any) {
	c.JSON(status, response.Response{Code: response.SUCCESS, Data: data, Msg: "成功"})
}

func fail(c *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, service.ErrTooLarge), errors.As(err, &tooLarge):
		Fail(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "请求超过大小限制")
	case errors.Is(err, service.ErrInvalid):
		Fail(c, http.StatusBadRequest, "INVALID_INPUT", "请求参数无效")
	case errors.Is(err, service.ErrUnauthorized):
		Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "登录凭据无效或已过期")
	case errors.Is(err, service.ErrForbidden):
		Fail(c, http.StatusForbidden, "FORBIDDEN", "无权执行此操作")
	case errors.Is(err, service.ErrNotFound):
		Fail(c, http.StatusNotFound, "NOT_FOUND", "资源不存在")
	case errors.Is(err, service.ErrIntentReused):
		Fail(c, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "请求键已用于不同的请求")
	case errors.Is(err, service.ErrIntentInProgress):
		Fail(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "请求正在处理中，请用相同请求键重试")
	case errors.Is(err, service.ErrStaleTask):
		Fail(c, http.StatusConflict, "STALE_TASK", "任务状态已变更，请刷新")
	case errors.Is(err, service.ErrRouteUnavailable):
		Fail(c, http.StatusConflict, "ROUTE_UNAVAILABLE", "审批路径不可用")
	case errors.Is(err, service.ErrTransferLimit):
		Fail(c, http.StatusConflict, "TRANSFER_LIMIT", "已达到转交次数上限")
	case errors.Is(err, service.ErrStaleRecord):
		Fail(c, http.StatusConflict, "STALE_RECORD", "记录已变更，请刷新")
	case errors.Is(err, service.ErrIntegrationDisabled):
		Fail(c, http.StatusServiceUnavailable, "INTEGRATION_DISABLED", "集成未启用")
	case errors.Is(err, service.ErrInvalidState):
		Fail(c, http.StatusConflict, "INVALID_STATE", "当前状态不允许此操作")
	case errors.Is(err, service.ErrAccountConflict):
		Fail(c, http.StatusConflict, "ACCOUNT_CONFLICT", "账号名称或绑定冲突")
	case errors.Is(err, service.ErrHandoffRequired):
		Fail(c, http.StatusConflict, "HANDOFF_REQUIRED", "请指定审批交接人")
	case errors.Is(err, service.ErrHandoffConflict):
		Fail(c, http.StatusConflict, "HANDOFF_CONFLICT", "交接人不可用或与申请冲突")
	case errors.Is(err, service.ErrLeaveOverlap):
		Fail(c, http.StatusConflict, "LEAVE_OVERLAP", "请假时间与已有申请重叠")
	case errors.Is(err, service.ErrInsufficientLeave):
		Fail(c, http.StatusConflict, "INSUFFICIENT_LEAVE", "年度可用额度不足")
	case errors.Is(err, service.ErrRuleConflict):
		Fail(c, http.StatusConflict, "RULE_CONFLICT", "审批规则与同优先级规则重叠")
	case errors.Is(err, service.ErrReminderRateLimit):
		Fail(c, http.StatusTooManyRequests, "REMINDER_RATE_LIMIT", "请间隔五分钟再催办")
	case errors.Is(err, service.ErrImportInvalid):
		Fail(c, http.StatusConflict, "IMPORT_INVALID", "预览含错误或没有可导入数据")
	case errors.Is(err, service.ErrImportConflict):
		Fail(c, http.StatusConflict, "IMPORT_CONFLICT", "部门或工号已变更，请重新预览")
	case errors.Is(err, service.ErrPreviewExpired):
		Fail(c, http.StatusGone, "PREVIEW_EXPIRED", "导入预览已过期")
	case errors.Is(err, service.ErrExportLimit):
		Fail(c, http.StatusConflict, "EXPORT_LIMIT", "授权导出记录超过上限")
	case errors.Is(err, service.ErrConflict):
		Fail(c, http.StatusConflict, "DUPLICATE_IDENTIFIER", "编号已存在")
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "操作失败")
	}
}

func readJSON(c *gin.Context, input any) bool {
	return readBoundedJSON(c, input, maxJSONSize)
}

func readBoundedJSON(c *gin.Context, input any, maxBytes int64) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		fail(c, service.ErrInvalid)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		failJSON(c, err)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		failJSON(c, err)
		return false
	}
	return true
}

func failJSON(c *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		fail(c, err)
		return
	}
	fail(c, service.ErrInvalid)
}

func resourceID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		fail(c, service.ErrInvalid)
		return 0, false
	}
	return uint(id), true
}

func pageParams(c *gin.Context) (model.Pagination, bool) {
	p := model.Pagination{Page: 1, PageSize: 20}
	if !readPageNumber(c, "page", &p.Page) || !readPageNumber(c, "pageSize", &p.PageSize) ||
		p.PageSize > 100 || p.Page-1 > math.MaxInt/p.PageSize {
		fail(c, service.ErrInvalid)
		return p, false
	}
	return p, true
}

func readPageNumber(c *gin.Context, name string, destination *int) bool {
	values, exists := c.GetQueryArray(name)
	if !exists {
		return true
	}
	if len(values) != 1 {
		return false
	}
	number, err := strconv.Atoi(values[0])
	if err != nil || number < 1 {
		return false
	}
	*destination = number
	return true
}
