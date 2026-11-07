package api

import (
	"context"
	"net/http"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

const (
	actorKey   = "oa.actor"
	sessionKey = "oa.session"
)

type healthView struct {
	Status          string `json:"status"`
	Database        string `json:"database"`
	IntegrationMode string `json:"integrationMode"`
}

// Health godoc
// @Tags OA Health
// @Summary 检查开发服务和数据库
// @Produce json
// @Success 200 {object} response.Response{data=healthView}
// @Failure 503 {object} response.Response{data=errorData}
// @Router /api/oa/v1/health [get]
func (a *API) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := a.service.Health(ctx); err != nil {
		Fail(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "数据库不可用")
		return
	}
	succeed(c, http.StatusOK, healthView{Status: "ok", Database: "ok", IntegrationMode: "local-development"})
}

// Login godoc
// @Tags OA Auth
// @Summary 使用本地开发账户登录
// @Accept json
// @Produce json
// @Param credentials body model.LoginInput true "开发账户和密码"
// @Success 200 {object} response.Response{data=model.LoginView}
// @Failure 400,401,403,413,429,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/auth/login [post]
func (a *API) Login(c *gin.Context) {
	var input model.LoginInput
	if !readJSON(c, &input) {
		return
	}
	result, err := a.service.Login(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusOK, result)
}

func (a *API) Authenticate(c *gin.Context) {
	values := c.Request.Header.Values("x-token")
	if len(values) != 1 {
		fail(c, service.ErrUnauthorized)
		return
	}
	actor, session, err := a.service.Authenticate(c.Request.Context(), values[0])
	if err != nil {
		fail(c, err)
		return
	}
	c.Set(actorKey, actor)
	c.Set(sessionKey, session)
	c.Next()
}

func Actor(c *gin.Context) model.Actor { return c.MustGet(actorKey).(model.Actor) }

// Me godoc
// @Tags OA Auth
// @Summary 获取当前用户及权限
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.SessionView}
// @Failure 401,403,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/auth/me [get]
func (a *API) Me(c *gin.Context) { succeed(c, http.StatusOK, c.MustGet(sessionKey)) }
