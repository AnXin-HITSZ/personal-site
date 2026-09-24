package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			log.Printf("panic recovered: %v\n%s", recovered, debug.Stack())

			if c.Writer.Written() {
				c.Abort()
				return
			}

			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewInternalError())
		}()

		c.Next()
	}
}
