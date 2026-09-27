package middleware

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 只记 5xx。4xx 是客户端的问题，量可以很大（扫描器、探针、爬虫），记下来只会把
// 有价值的行淹掉；而 5xx 是服务端自己的问题，每一条都应该有人看到。
//
// 这个中间件要注册在 Recovery 外面：里面那个 defer 才看得到 Recovery 已经改写成
// 500 的状态码，崩溃请求才会留下一条带路径的记录。
func LogServerErrors() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			status := c.Writer.Status()
			if status >= http.StatusInternalServerError {
				logServerError(c, status)
			}
		}()

		c.Next()
	}
}

func logServerError(c *gin.Context, status int) {
	// 路径不带查询串是有意的：重置口令的令牌就在查询串里。请求体同理，里面有明文口令。
	// 路径和 User-Agent 都由客户端控制，%q 会把换行转义掉——不转义的话，构造一个含
	// 换行的 UA 就能往日志里插一整行假记录。
	var builder strings.Builder
	fmt.Fprintf(&builder, "HTTP %d %s %q client=%s ua=%q",
		status, c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent())

	// 状态码只说明「坏了」，说明不了「为什么」。原因由 handler 用 c.Error 挂上来。
	for _, attached := range c.Errors {
		fmt.Fprintf(&builder, " err=%q", attached.Err.Error())
	}

	log.Print(builder.String())
}
