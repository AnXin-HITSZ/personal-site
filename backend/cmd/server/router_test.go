package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"anxin-hitsz.com/backend/internal/config"
	"anxin-hitsz.com/backend/internal/dto"
)

func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{SkipInitializeWithVersion: true}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("构造 DryRun 连接失败：%v", err)
	}
	return db
}

// deploy/start.sh 的就绪探针是一个不带任何请求头、也不带 cookie 的匿名
// GET /api/v1/articles，拿到 200 才算启动成功。网关和鉴权都不能拦它。
//
// 这里断言的是「没被 403/401 挡在门口」而不是「等于 200」：DryRun 下
// Transaction 开不出来，业务处理必然失败，但失败发生在中间件之后——
// 那正是这个测试要证明的位置关系。
func TestReadinessProbeShapeIsNotBlockedByGateway(t *testing.T) {
	router, err := newRouter(config.Config{
		AppEnv:  "production",
		Session: config.SessionConfig{CookieSecure: true},
	}, dryRunDB(t))
	if err != nil {
		t.Fatalf("构造路由失败：%v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/articles", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("匿名 GET 不该被网关或鉴权拦下，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

func TestSessionRouteRequiresLogin(t *testing.T) {
	router, err := newRouter(config.Config{
		AppEnv:  "production",
		Session: config.SessionConfig{CookieSecure: true},
	}, dryRunDB(t))
	if err != nil {
		t.Fatalf("构造路由失败：%v", err)
	}

	cases := []struct {
		name   string
		cookie string
	}{
		{name: "没有 cookie"},
		{name: "cookie 是个编造的令牌", cookie: "axh_sid=forged-token"},
		{name: "cookie 名不对", cookie: "session=forged-token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/account/session", nil)
			if tc.cookie != "" {
				req.Header.Set("Cookie", tc.cookie)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("应返回 401，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), dto.CodeUnauthorized) {
				t.Errorf("响应体里应含 %s：%s", dto.CodeUnauthorized, rec.Body.String())
			}
		})
	}
}
