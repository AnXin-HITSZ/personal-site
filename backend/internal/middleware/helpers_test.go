package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type call struct {
	method  string
	target  string
	headers map[string]string
	cookie  string
}

func send(router *gin.Engine, c call) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.target, nil)
	for name, value := range c.headers {
		req.Header.Set(name, value)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", "axh_sid="+c.cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body dto.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	return body.Error.Code
}

func okHandler(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
