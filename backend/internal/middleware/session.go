package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/service"
)

const sessionContextKey = "axh.session"

type SessionCookie struct {
	Name   string
	Secure bool
	MaxAge int
}

func NewSessionCookie(secure bool) SessionCookie {
	return SessionCookie{
		Name:   service.SessionCookieName,
		Secure: secure,
		MaxAge: int(service.SessionTTL / time.Second),
	}
}

// gin 的 SetCookie 没有 SameSite 参数，只能先调 SetSameSite 单独设一次。漏掉这一步
// 不会报任何错，只是 cookie 上根本没有 SameSite 属性——浏览器的第一层 CSRF 防护
// 就空了。HttpOnly 同理，走 SetCookie 的最后一个参数。
func (c SessionCookie) Set(ctx *gin.Context, token string) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(c.Name, token, c.MaxAge, "/", "", c.Secure, true)
}

// MaxAge 传负数，Go 会写出 Max-Age=0 加一个过期的 Expires，浏览器立刻删掉它。
func (c SessionCookie) Clear(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(c.Name, "", -1, "/", "", c.Secure, true)
}

func (c SessionCookie) Read(ctx *gin.Context) string {
	token, err := ctx.Cookie(c.Name)
	if err != nil {
		return ""
	}
	return token
}

func RequireAuth(cookies SessionCookie, sessions *service.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := cookies.Read(c)

		account, err := sessions.Authenticate(c.Request.Context(), token)
		if errors.Is(err, service.ErrSessionInvalid) {
			// 浏览器手里那个令牌已经没用了，顺手清掉，免得它每次都带上来。
			if token != "" {
				cookies.Clear(c)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.NewUnauthorized())
			return
		}
		if err != nil {
			// 查会话失败被当成 401 处理，就等于数据库一抖动所有人都被登出，
			// 而且第二天没人能从日志里看出发生过这件事。
			_ = c.Error(err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewInternalError())
			return
		}

		c.Set(sessionContextKey, account)
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		account, ok := CurrentAccount(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.NewUnauthorized())
			return
		}
		if account.User.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.NewForbidden("没有权限"))
			return
		}
		c.Next()
	}
}

func CurrentAccount(c *gin.Context) (*service.Authenticated, bool) {
	value, ok := c.Get(sessionContextKey)
	if !ok {
		return nil, false
	}
	account, ok := value.(*service.Authenticated)
	return account, ok
}
