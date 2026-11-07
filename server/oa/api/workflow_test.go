package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

func TestIntentReplayPreservesHTTPStatusAndJSON(t *testing.T) {
	original := service.IntentResult{Status: http.StatusCreated, Body: json.RawMessage(`{"id":1,"form":{"content":"\u003ctest\u003e"}}`)}
	send := func(result service.IntentResult) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		sendIntent(c, result, nil)
		return recorder
	}
	first := send(original)
	original.Replayed = true
	second := send(original)
	if first.Code != http.StatusCreated || second.Code != first.Code || second.Body.String() != first.Body.String() ||
		first.Header().Get("Idempotency-Replayed") != "" || second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay differs from original response or has incorrect replay header")
	}
}
