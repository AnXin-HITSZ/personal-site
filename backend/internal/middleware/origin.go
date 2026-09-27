package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

// 头名是 jQuery 时代留下来的老约定，取它只是因为「跨站发不出自定义头」这一点；
// 叫什么名字无所谓，但改这里必须同步改前端。
const (
	requestedWithHeader = "X-Requested-With"
	requestedWithValue  = "XMLHttpRequest"
)

type OriginPolicy struct {
	Allowed []string
}

func NewOriginPolicy(allowed []string) OriginPolicy {
	cleaned := make([]string, 0, len(allowed))
	for _, origin := range allowed {
		if trimmed := strings.TrimRight(strings.TrimSpace(origin), "/"); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return OriginPolicy{Allowed: cleaned}
}

func (p OriginPolicy) permits(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	// 同源：Origin 里的主机就是这次请求打到的主机。反向代理必须原样保留 Host——
	// nginx 的 $host 保留了，而 Vite 的 proxy 因为是字符串写法被自动加上
	// changeOrigin:true，会把 Host 改写成 127.0.0.1:8080，所以开发环境要靠
	// ALLOWED_ORIGINS 补上 http://localhost:5173。
	//
	// 这里只比主机不比协议：能发出 Origin 的只有浏览器，而浏览器的 Origin 是
	// 页面真实的来源；http 版本的站点已被 nginx 301 到 https，没有页面能发出
	// http 的 Origin。这条依赖记在这里，改 nginx 那段 301 时要想到它。
	if strings.EqualFold(parsed.Host, requestHost) {
		return true
	}

	for _, allowed := range p.Allowed {
		if strings.EqualFold(strings.TrimRight(origin, "/"), allowed) {
			return true
		}
	}
	return false
}

// 只拦会改状态的方法。GET/HEAD 不该改任何东西，也就没什么可防的。
func (p OriginPolicy) SameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}

		// 第一层：跨站请求发不出自定义头。加上它请求就不再是「简单请求」，
		// 浏览器会先发一次 CORS 预检，而本服务不返回任何 Access-Control-Allow-*，
		// 预检过不去，真正的请求根本不会发出来。
		if c.GetHeader(requestedWithHeader) != requestedWithValue {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.NewForbidden("缺少 "+requestedWithHeader+" 请求头"))
			return
		}

		// 第二层：不用 Content-Type 代替这一层。gin 的 ShouldBindJSON 压根不看
		// Content-Type，text/plain 的请求体照样会被当 JSON 解析，指望框架拦住
		// 是拦不住的。
		if !p.permits(c.GetHeader("Origin"), c.Request.Host) {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.NewForbidden("请求来源不被允许"))
			return
		}

		c.Next()
	}
}
