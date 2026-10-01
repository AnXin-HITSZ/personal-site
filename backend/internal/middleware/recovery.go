package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

// 自己写一个而不用 gin 自带的 Recovery，是为了让崩溃的响应也走本站的信封
// （dto.NewInternalError）——前端拿到的错误形状因此始终是同一种。
//
// 它要注册在 LogServerErrors 里面（先 Use 的在外层）：外层那个 defer 才看得到这里
// 改写成的 500，崩溃的请求才会在日志里留下一条带路径的记录。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			log.Printf("panic recovered: %v\n%s", recovered, debug.Stack())

			// 响应头已经发出去了就改不了状态码，这时只能中止：再写一次 JSON 会接在
			// 已经出去的字节后面，把响应体弄成两截。
			if c.Writer.Written() {
				c.Abort()
				return
			}

			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewInternalError())
		}()

		c.Next()
	}
}
