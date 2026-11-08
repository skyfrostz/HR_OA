package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/api"
)

func TestCasbinPolicies(t *testing.T) {
	rbac, err := newPolicy(protectedRoutes(api.New(nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		role, route, method string
		allowed             bool
	}{
		{"HR_ADMIN", "/employees", "POST", true},
		{"HR_ADMIN", "/employees/:id/contracts", "POST", true},
		{"HR_ADMIN", "/contracts/:id/files", "POST", true},
		{"HR_VIEWER", "/employees", "POST", false},
		{"HR_VIEWER", "/employees/:id/contracts", "POST", false},
		{"HR_VIEWER", "/contracts/:id/files", "POST", false},
		{"EMPLOYEE", "/employees", "POST", false},
		{"EMPLOYEE", "/employees/:id/contracts", "POST", false},
		{"EMPLOYEE", "/contracts/:id/files", "POST", false},
		{"HR_VIEWER", "/files/:id/download", "GET", true},
		{"EMPLOYEE", "/employees/:id/contracts", "GET", true},
		{"APPROVER", "/employees", "GET", true},
		{"APPROVER", "/employees", "POST", false},
		{"APPROVER", "/contracts/:id/files", "POST", false},
		{"APPROVER", "/tasks/:id/actions", "POST", true},
		{"APPROVER", "/tasks/:id/transfer-targets", "GET", true},
		{"EMPLOYEE", "/requests", "POST", true},
		{"EMPLOYEE", "/tasks/:id/actions", "POST", false},
		{"HR_ADMIN", "/requests", "POST", false},
		{"HR_ADMIN", "/tasks/:id/actions", "POST", false},
		{"HR_VIEWER", "/request-templates", "GET", false},
		{"HR_VIEWER", "/requests", "GET", true},
		{"HR_ADMIN", "/departments/:id", "PUT", true},
		{"HR_VIEWER", "/departments", "GET", true},
		{"HR_VIEWER", "/departments/:id", "PUT", false},
		{"APPROVER", "/departments", "GET", false},
		{"HR_ADMIN", "/employee-imports/:id/confirm", "POST", true},
		{"HR_VIEWER", "/employee-imports/:id/confirm", "POST", false},
		{"HR_ADMIN", "/employee-imports/:id/discard", "POST", true},
		{"HR_VIEWER", "/employee-imports/:id/discard", "POST", false},
		{"EMPLOYEE", "/employee-imports/:id/discard", "POST", false},
		{"APPROVER", "/employee-imports/:id/discard", "POST", false},
		{"EMPLOYEE", "/employees/:id/annual-leave", "GET", true},
		{"EMPLOYEE", "/employees/:id/annual-leave/:year", "PUT", false},
		{"EMPLOYEE", "/employees/export.xlsx", "GET", true},
		{"EMPLOYEE", "/employees/search", "POST", true},
		{"APPROVER", "/employees/search", "POST", true},
		{"HR_VIEWER", "/employees/search", "POST", true},
		{"HR_ADMIN", "/employees/search", "POST", true},
		{"HR_VIEWER", "/employees/export.xlsx", "POST", true},
		{"EMPLOYEE", "/employees/export.xlsx", "POST", true},
		{"", "/employees/search", "POST", false},
		{"APPROVER", "/contract-reminders", "GET", true},
		{"HR_ADMIN", "/employees/:id", "DELETE", false},
		{"HR_ADMIN", "/init/initDB", "POST", false},
		{"", "/employees", "GET", false},
	} {
		allowed, err := rbac.Enforce(test.role, prefix+test.route, test.method)
		if err != nil || allowed != test.allowed {
			t.Errorf("%s %s %s got %t, error %v, want %t", test.role, test.method, test.route, allowed, err, test.allowed)
		}
	}
}

func TestOnlyHeaderAuthAndAllowlistedPaths(t *testing.T) {
	engine, err := New(api.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path   string
		status int
	}{
		{prefix + "/files/1/download?x-token=ignored", http.StatusUnauthorized},
		{prefix + "/employees/", http.StatusNotFound},
		{"/uploads/file/opaque", http.StatusNotFound},
		{"/init/checkDB", http.StatusNotFound},
		{"/autoCode/getTables", http.StatusNotFound},
		{"/plugins/install", http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(&http.Cookie{Name: "x-token", Value: "ignored"})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" ||
			response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("unexpected response for %s, status %d", test.path, response.Code)
		}
	}
}

func TestLoginRateLimitDoesNotTrustForwardedIP(t *testing.T) {
	engine, err := New(api.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	var rejected bool
	for i := 1; i <= 25; i++ {
		request := httptest.NewRequest(http.MethodPost, prefix+"/auth/login", strings.NewReader("{"))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", "192.0.2."+strconv.Itoa(i))
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code == http.StatusTooManyRequests {
			if response.Header().Get("Retry-After") == "" {
				t.Fatal("rate limited response lacks Retry-After")
			}
			rejected = true
			break
		}
		if response.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 before rate limit, got %d", response.Code)
		}
	}
	if !rejected {
		t.Fatal("spoofed forwarded IPs bypassed the login rate limit")
	}
}
