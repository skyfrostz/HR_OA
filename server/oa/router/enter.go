package router

import (
	"io"
	"net/http"
	"time"

	"github.com/casbin/casbin/v3"
	policy "github.com/casbin/casbin/v3/model"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/api"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	"github.com/gin-gonic/gin"
)

const (
	prefix      = "/api/oa/v1"
	policyModel = `
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act
`
)

type routeAccess uint8

const (
	authenticated routeAccess = iota
	hrWrite
	hrRead
	workflowSubmit
	workflowAct
)

type route struct {
	method  string
	path    string
	handler gin.HandlerFunc
	access  routeAccess
}

func protectedRoutes(a *api.API) []route {
	return []route{
		{http.MethodGet, "/auth/me", a.Me, authenticated},
		{http.MethodGet, "/employees", a.ListEmployees, authenticated},
		{http.MethodPost, "/employees", a.CreateEmployee, hrWrite},
		{http.MethodGet, "/employees/:id", a.Employee, authenticated},
		{http.MethodGet, "/employees/:id/contracts", a.ListContracts, authenticated},
		{http.MethodPost, "/employees/:id/contracts", a.CreateContract, hrWrite},
		{http.MethodPost, "/contracts/:id/files", a.UploadFile, hrWrite},
		{http.MethodGet, "/files/:id/download", a.DownloadFile, authenticated},
		{http.MethodGet, "/request-templates", a.RequestTemplates, workflowSubmit},
		{http.MethodPost, "/requests", a.SubmitRequest, workflowSubmit},
		{http.MethodGet, "/requests", a.MyRequests, authenticated},
		{http.MethodGet, "/requests/:id", a.RequestDetail, authenticated},
		{http.MethodGet, "/tasks", a.PendingTasks, authenticated},
		{http.MethodGet, "/tasks/history", a.TaskHistory, authenticated},
		{http.MethodGet, "/tasks/:id/transfer-targets", a.TransferTargets, workflowAct},
		{http.MethodPost, "/tasks/:id/actions", a.ActOnTask, workflowAct},
		{http.MethodGet, "/notifications", a.Notifications, authenticated},
		{http.MethodGet, "/departments", a.Departments, hrRead},
		{http.MethodPost, "/departments", a.CreateDepartment, hrWrite},
		{http.MethodPut, "/departments/:id", a.EditDepartment, hrWrite},
		{http.MethodPut, "/employees/:id", a.EditEmployee, hrWrite},
		{http.MethodGet, "/employees/:id/annual-leave", a.AnnualLeave, authenticated},
		{http.MethodPut, "/employees/:id/annual-leave/:year", a.WriteAnnualLeave, hrWrite},
		{http.MethodGet, "/employees/import-template.xlsx", a.EmployeeImportTemplate, hrWrite},
		{http.MethodPost, "/employee-imports/preview", a.PreviewEmployeeImport, hrWrite},
		{http.MethodGet, "/employee-imports/:id", a.EmployeeImportPreview, hrWrite},
		{http.MethodPost, "/employee-imports/:id/confirm", a.ConfirmEmployeeImport, hrWrite},
		{http.MethodGet, "/employees/export.xlsx", a.ExportEmployees, authenticated},
		{http.MethodGet, "/contract-reminders", a.ContractReminders, authenticated},
	}
}

func (access routeAccess) allows(role uint) bool {
	switch access {
	case authenticated:
		return true
	case hrWrite:
		return role == model.HRAdmin
	case hrRead:
		return role == model.HRAdmin || role == model.HRViewer
	case workflowSubmit:
		return role == model.EmployeeRole || role == model.ApproverRole
	case workflowAct:
		return role == model.ApproverRole
	default:
		return false
	}
}

func New(a *api.API) (*gin.Engine, error) {
	routes := protectedRoutes(a)
	rbac, err := newPolicy(routes)
	if err != nil {
		return nil, err
	}

	engine := gin.New()
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false
	if err := engine.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	engine.Use(securityHeaders, gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		api.Fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "操作失败")
	}))
	engine.NoRoute(func(c *gin.Context) {
		api.Fail(c, http.StatusNotFound, "NOT_FOUND", "资源不存在")
	})

	oa := engine.Group(prefix)
	oa.GET("/health", a.Health)
	oa.POST("/auth/login", newLoginLimiter().handle, a.Login)
	protected := oa.Group("", a.Authenticate, authorize(rbac))
	for _, endpoint := range routes {
		protected.Handle(endpoint.method, endpoint.path, endpoint.handler)
	}
	return engine, nil
}

func securityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "no-referrer")
	c.Next()
}

func newPolicy(routes []route) (*casbin.SyncedEnforcer, error) {
	m, err := policy.NewModelFromString(policyModel)
	if err != nil {
		return nil, err
	}
	rbac, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range routes {
		for _, role := range []uint{model.HRAdmin, model.HRViewer, model.EmployeeRole, model.ApproverRole} {
			if !endpoint.access.allows(role) {
				continue
			}
			if _, err := rbac.AddPolicy(model.RoleName(role), prefix+endpoint.path, endpoint.method); err != nil {
				return nil, err
			}
		}
	}
	return rbac, nil
}

func authorize(rbac *casbin.SyncedEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := model.RoleName(api.Actor(c).AuthorityID)
		allowed, err := rbac.Enforce(role, c.FullPath(), c.Request.Method)
		if err != nil {
			api.Fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "操作失败")
			return
		}
		if !allowed {
			api.Fail(c, http.StatusForbidden, "FORBIDDEN", "无权执行此操作")
			return
		}
		c.Next()
	}
}

func (l *loginLimiter) handle(c *gin.Context) {
	if !l.allow(c.ClientIP(), time.Now()) {
		c.Header("Retry-After", "6")
		api.Fail(c, http.StatusTooManyRequests, "RATE_LIMITED", "登录过于频繁，请稍后重试")
		return
	}
	c.Next()
}
