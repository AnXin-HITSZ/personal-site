package middleware

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

// log 是包级的，测试里只能临时换掉输出再换回来。这里不并行跑，
// 否则两个测试会互相抢同一个 Writer。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(original) })
	return &buffer
}

func errorLogRouter(register func(*gin.Engine)) *gin.Engine {
	router := gin.New()
	// 和 main.go 里一样的顺序：记录者在 Recovery 外面。
	router.Use(LogServerErrors(), Recovery())
	register(router)
	return router
}

func TestLogServerErrorsIgnoresNonServerFailures(t *testing.T) {
	router := errorLogRouter(func(router *gin.Engine) {
		router.GET("/ok", okHandler)
		router.GET("/bad-request", func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("page", "分页参数不合法"))
		})
		router.GET("/forbidden", func(c *gin.Context) {
			c.JSON(http.StatusForbidden, dto.NewForbidden("没有权限"))
		})
		router.GET("/unauthorized", func(c *gin.Context) {
			c.JSON(http.StatusUnauthorized, dto.NewUnauthorized())
		})
	})

	cases := []call{
		{method: http.MethodGet, target: "/ok"},
		{method: http.MethodGet, target: "/bad-request"},
		{method: http.MethodGet, target: "/forbidden"},
		{method: http.MethodGet, target: "/unauthorized"},
		{method: http.MethodGet, target: "/no-such-route"},
		{method: http.MethodPost, target: "/ok"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			buffer := captureLog(t)
			rec := send(router, tc)

			if got := buffer.String(); got != "" {
				t.Errorf("状态码 %d 不该记日志，实际写了：%s", rec.Code, got)
			}
		})
	}
}

func TestLogServerErrorsRecordsServerFailure(t *testing.T) {
	router := errorLogRouter(func(router *gin.Engine) {
		router.GET("/boom", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		})
	})

	buffer := captureLog(t)
	rec := send(router, call{
		method:  http.MethodGet,
		target:  "/boom",
		headers: map[string]string{"User-Agent": "curl/8.5.0"},
	})

	// 中间件只负责记录，不能顺手改掉响应。
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码应保持 500，实际 %d", rec.Code)
	}
	if code := errorCode(t, rec); code != dto.CodeInternalError {
		t.Errorf("响应体应仍是 %s，实际 %s", dto.CodeInternalError, code)
	}

	logged := buffer.String()
	for _, want := range []string{"HTTP 500", "GET", `"/boom"`, "client=192.0.2.1", `ua="curl/8.5.0"`} {
		if !strings.Contains(logged, want) {
			t.Errorf("日志里应含 %s，实际：%s", want, logged)
		}
	}
	if strings.Count(logged, "HTTP 500") != 1 {
		t.Errorf("一次失败应只留一行，实际：%s", logged)
	}
}

// 状态码只说明「坏了」。真正让人少猜半小时的是被 handler 挂上来的原因，
// 而中间件看不到 handler 手里的 err，只能靠 c.Error 传出来。
func TestLogServerErrorsCarriesAttachedCause(t *testing.T) {
	router := errorLogRouter(func(router *gin.Engine) {
		router.GET("/boom", func(c *gin.Context) {
			_ = c.Error(errors.New("Error 1146: Table 'sessions' doesn't exist"))
			c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		})
	})

	buffer := captureLog(t)
	send(router, call{method: http.MethodGet, target: "/boom"})

	if !strings.Contains(buffer.String(), "1146") {
		t.Errorf("日志里应带上挂载的原因，实际：%s", buffer.String())
	}
}

// 重置口令的令牌在查询串里。日志是给人看的，也可能是被别人看到的。
func TestLogServerErrorsOmitsQueryString(t *testing.T) {
	const token = "super-secret-reset-token"
	router := errorLogRouter(func(router *gin.Engine) {
		router.GET("/reset", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		})
	})

	buffer := captureLog(t)
	send(router, call{method: http.MethodGet, target: "/reset?token=" + token})

	logged := buffer.String()
	if strings.Contains(logged, token) {
		t.Errorf("日志不该带查询串，实际：%s", logged)
	}
	if !strings.Contains(logged, `"/reset"`) {
		t.Errorf("路径本身仍应记下来，实际：%s", logged)
	}
}

// 路径和 User-Agent 都由客户端控制。不转义换行的话，构造一个含换行的 UA
// 就能往日志里插一整行看起来像真记录的假记录。
func TestLogServerErrorsEscapesClientControlledNewlines(t *testing.T) {
	router := errorLogRouter(func(router *gin.Engine) {
		// 带参数的路由才收得下路径里的换行：gin 是按解码后的路径匹配的，
		// `/boom%0aX` 根本匹配不到 `/boom`，只会得到一个 404。
		router.GET("/boom/:slug", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		})
	})

	buffer := captureLog(t)
	send(router, call{
		method:  http.MethodGet,
		target:  "/boom/ok",
		headers: map[string]string{"User-Agent": "evil\nHTTP 500 GET /fake\n"},
	})
	rec := send(router, call{method: http.MethodGet, target: "/boom/a%0ab"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("构造前提不成立：状态码 %d", rec.Code)
	}

	// 换行若被原样写进去，一次请求就会裂成多行，伪造的行会顶到行首。
	// 真实记录都以 log 的时间戳开头，所以「以 HTTP 500 开头」就是被插进来的。
	lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("两次请求应只留两行，实际 %d 行：\n%s", len(lines), buffer.String())
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "HTTP 500") {
			t.Errorf("伪造的记录行没有被转义：%s", line)
		}
	}
}

// 这条钉住的是 main.go 里的注册顺序：LogServerErrors 必须在 Recovery 外面，
// 否则崩溃请求在它眼里状态码还是 200，什么都不会记。
func TestLogServerErrorsRecordsRecoveredPanic(t *testing.T) {
	router := errorLogRouter(func(router *gin.Engine) {
		router.GET("/panic", func(c *gin.Context) {
			panic("数据库连接池空了")
		})
	})

	buffer := captureLog(t)
	rec := send(router, call{method: http.MethodGet, target: "/panic"})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Recovery 应把它变成 500，实际 %d", rec.Code)
	}

	logged := buffer.String()
	if !strings.Contains(logged, "HTTP 500") || !strings.Contains(logged, `"/panic"`) {
		t.Errorf("崩溃请求也该留下带路径的记录，实际：%s", logged)
	}
	if !strings.Contains(logged, "panic recovered") {
		t.Errorf("Recovery 自己的堆栈记录不应被取代，实际：%s", logged)
	}
}
