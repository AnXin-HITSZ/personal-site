package middleware

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

const (
	siteURL  = "https://anxin-hitsz.com"
	ajaxHead = "XMLHttpRequest"
)

func newOriginRouter(policy OriginPolicy) *gin.Engine {
	router := gin.New()
	router.Use(policy.SameOrigin())
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete} {
		router.Handle(method, "/api/v1/thing", okHandler)
	}
	return router
}

func ajax(origin string) map[string]string {
	return map[string]string{requestedWithHeader: ajaxHead, "Origin": origin}
}

func TestSameOriginGuardsEveryMutatingMethod(t *testing.T) {
	router := newOriginRouter(NewOriginPolicy(nil))

	cases := []struct {
		name string
		call call
		want int
	}{
		{
			name: "GET 不拦",
			call: call{method: http.MethodGet, target: siteURL + "/api/v1/thing"},
			want: http.StatusOK,
		},
		{
			name: "HEAD 不拦",
			call: call{method: http.MethodHead, target: siteURL + "/api/v1/thing"},
			want: http.StatusOK,
		},
		{
			name: "GET 甚至连跨站来源都不拦",
			call: call{method: http.MethodGet, target: siteURL + "/api/v1/thing", headers: map[string]string{"Origin": "https://evil.example"}},
			want: http.StatusOK,
		},
		{
			name: "同源 POST 带自定义头",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax(siteURL)},
			want: http.StatusOK,
		},
		{
			name: "缺自定义头",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: map[string]string{"Origin": siteURL}},
			want: http.StatusForbidden,
		},
		{
			name: "自定义头的值不对",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: map[string]string{requestedWithHeader: ajaxHead + "2", "Origin": siteURL}},
			want: http.StatusForbidden,
		},
		{
			name: "缺 Origin",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: map[string]string{requestedWithHeader: ajaxHead}},
			want: http.StatusForbidden,
		},
		{
			name: "跨站来源",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("https://evil.example")},
			want: http.StatusForbidden,
		},
		{
			name: "前缀伪装",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("https://anxin-hitsz.com.evil.example")},
			want: http.StatusForbidden,
		},
		{
			name: "后缀伪装",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("https://evil-anxin-hitsz.com")},
			want: http.StatusForbidden,
		},
		{
			name: "null 来源（沙箱 iframe 与 data: 页面发的就是它）",
			call: call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("null")},
			want: http.StatusForbidden,
		},
		{
			name: "PUT 同样受保护",
			call: call{method: http.MethodPut, target: siteURL + "/api/v1/thing", headers: map[string]string{"Origin": siteURL}},
			want: http.StatusForbidden,
		},
		{
			name: "DELETE 同样受保护",
			call: call{method: http.MethodDelete, target: siteURL + "/api/v1/thing"},
			want: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := send(router, tc.call); rec.Code != tc.want {
				t.Errorf("应返回 %d，实际 %d：%s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSameOriginTreatsHostCaseInsensitively(t *testing.T) {
	router := newOriginRouter(NewOriginPolicy(nil))
	rec := send(router, call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("https://ANXIN-HITSZ.COM")})
	if rec.Code != http.StatusOK {
		t.Errorf("主机名大小写不该影响判断，实际 %d", rec.Code)
	}
}

func TestSameOriginAcceptsWWWHost(t *testing.T) {
	router := newOriginRouter(NewOriginPolicy(nil))
	rec := send(router, call{method: http.MethodPost, target: "https://www.anxin-hitsz.com/api/v1/thing", headers: ajax("https://www.anxin-hitsz.com")})
	if rec.Code != http.StatusOK {
		t.Errorf("www 与请求的 Host 一致，应放行，实际 %d", rec.Code)
	}
}

// 开发环境的真实形状：浏览器在 http://localhost:5173，Vite 的 proxy 因为是字符串写法
// 被自动补上 changeOrigin:true，转发时把 Host 改写成 127.0.0.1:8080，于是 Origin 的
// 主机和请求的 Host 对不上，必须靠 ALLOWED_ORIGINS 补一条。
func TestOriginAllowlistCoversProxyRewrittenHost(t *testing.T) {
	devCall := call{
		method:  http.MethodPost,
		target:  "http://127.0.0.1:8080/api/v1/thing",
		headers: ajax("http://localhost:5173"),
	}

	cases := []struct {
		name    string
		allowed []string
		want    int
	}{
		{"不配许可名单时拒绝", nil, http.StatusForbidden},
		{"配了就放行", []string{"http://localhost:5173"}, http.StatusOK},
		{"末尾斜杠和大小写被容忍", []string{"http://LOCALHOST:5173/"}, http.StatusOK},
		{"多个来源用逗号分隔后逐条生效", []string{"https://staging.example", "http://localhost:5173"}, http.StatusOK},
		{"名单里没有的仍然拒绝", []string{"http://localhost:6000"}, http.StatusForbidden},
		{"空字符串被忽略，不会变成放行一切", []string{"", "   "}, http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := send(newOriginRouter(NewOriginPolicy(tc.allowed)), devCall); rec.Code != tc.want {
				t.Errorf("应返回 %d，实际 %d", tc.want, rec.Code)
			}
		})
	}
}

// 只比主机不比协议。这条的前提是 nginx 把 http 整站 301 到了 https（见
// deploy/nginx/anxin-hitsz.com.conf.example 的第一个 server 块）——没有页面能发出
// http 的 Origin。改那段 301 之前先想清楚这个测试在保护什么。
func TestOriginPolicyComparesHostNotScheme(t *testing.T) {
	router := newOriginRouter(NewOriginPolicy(nil))
	rec := send(router, call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("http://anxin-hitsz.com")})
	if rec.Code != http.StatusOK {
		t.Errorf("主机一致应放行，实际 %d", rec.Code)
	}
	// 端口不同就是不同的主机。
	other := send(router, call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("http://anxin-hitsz.com:8443")})
	if other.Code != http.StatusForbidden {
		t.Errorf("端口不同应拒绝，实际 %d", other.Code)
	}
}

func TestSameOriginRejectionUsesTheAppErrorEnvelope(t *testing.T) {
	router := newOriginRouter(NewOriginPolicy(nil))
	rec := send(router, call{method: http.MethodPost, target: siteURL + "/api/v1/thing", headers: ajax("https://evil.example")})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("应返回 403，实际 %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("错误响应应是 JSON，实际 %q", contentType)
	}
	if code := errorCode(t, rec); code != dto.CodeForbidden {
		t.Errorf("错误码应为 %s，实际 %s", dto.CodeForbidden, code)
	}
}
