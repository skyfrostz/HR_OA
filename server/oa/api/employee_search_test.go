package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmployeeSearchRejectsInvalidJSONBeforeQuery(t *testing.T) {
	a := New(nil)
	for _, handler := range []gin.HandlerFunc{a.SearchEmployees, a.ExportFilteredEmployees} {
		for _, body := range []string{"null", "[]", "{", `{"unexpected":true}`, `{"departmentId":-1}`, `{"departmentId":1.5}`, `{} {}`} {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			handler(c)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("invalid input accepted: %q status %d", body, recorder.Code)
			}
		}
	}
}
