package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
	"anxin-hitsz.com/backend/internal/service"
)

type fakeStore struct {
	sessions map[string]model.Session
	users    map[string]model.User
	lookups  []string
	failWith error
}

func (f *fakeStore) CreateSession(_ context.Context, session *model.Session) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.sessions[session.TokenHash] = *session
	return nil
}

func (f *fakeStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*model.Session, error) {
	f.lookups = append(f.lookups, tokenHash)
	if f.failWith != nil {
		return nil, f.failWith
	}
	session, ok := f.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &session, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	user, ok := f.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &user, nil
}

func (f *fakeStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	if f.failWith != nil {
		return f.failWith
	}
	delete(f.sessions, tokenHash)
	return nil
}

var _ service.SessionStore = (*fakeStore)(nil)

type authFixture struct {
	store    *fakeStore
	cookies  SessionCookie
	sessions *service.Session
	routes   func(*gin.Engine)
}

func newAuthFixture(secure bool) *authFixture {
	store := &fakeStore{
		sessions: map[string]model.Session{},
		users: map[string]model.User{
			"u1":    {ID: "u1", Email: "anxin@example.com", Role: model.RoleMember, Status: model.UserStatusActive},
			"admin": {ID: "admin", Email: "owner@example.com", Role: model.RoleAdmin, Status: model.UserStatusActive},
		},
	}
	cookies := NewSessionCookie(secure)
	sessions := service.NewSession(store)

	f := &authFixture{store: store, cookies: cookies, sessions: sessions}
	f.routes = func(router *gin.Engine) {
		router.GET("/api/v1/account/session", RequireAuth(cookies, sessions), func(c *gin.Context) {
			account, ok := CurrentAccount(c)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"missing": true})
				return
			}
			c.JSON(http.StatusOK, dto.NewSessionView(account.User, account.Session.ExpiresAt))
		})
		router.POST("/api/v1/admin/articles", RequireAuth(cookies, sessions), RequireRole(model.RoleAdmin), okHandler)
		router.POST("/api/v1/login", func(c *gin.Context) { cookies.Set(c, "test-token-value") })
		router.POST("/api/v1/logout", func(c *gin.Context) { cookies.Clear(c) })
	}
	return f
}

func (f *authFixture) router() *gin.Engine {
	router := gin.New()
	f.routes(router)
	return router
}

// 网关层的检查挂在最外层，和真实路由的组合顺序一致。
func (f *authFixture) routerWithOrigin(policy OriginPolicy) *gin.Engine {
	router := gin.New()
	router.Use(policy.SameOrigin())
	f.routes(router)
	return router
}

func (f *authFixture) startSession(t *testing.T, userID string) string {
	t.Helper()
	token, _, err := f.sessions.Start(context.Background(), userID, "Mozilla/5.0", "203.0.113.7")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	return token
}

func TestRequireAuthAcceptsLiveSession(t *testing.T) {
	f := newAuthFixture(false)
	token := f.startSession(t, "u1")

	rec := send(f.router(), call{method: http.MethodGet, target: "/api/v1/account/session", cookie: token})
	if rec.Code != http.StatusOK {
		t.Fatalf("有效会话应通过，实际 %d：%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "anxin@example.com") || !strings.Contains(body, `"role":"member"`) {
		t.Errorf("响应里没有账号信息：%s", body)
	}
}

func TestRequireAuthLooksUpByHashNotByToken(t *testing.T) {
	f := newAuthFixture(false)
	token := f.startSession(t, "u1")
	f.store.lookups = nil

	if rec := send(f.router(), call{method: http.MethodGet, target: "/api/v1/account/session", cookie: token}); rec.Code != http.StatusOK {
		t.Fatalf("有效会话应通过，实际 %d", rec.Code)
	}
	if len(f.store.lookups) != 1 {
		t.Fatalf("应只查一次会话表，实际 %d 次", len(f.store.lookups))
	}
	if f.store.lookups[0] == token {
		t.Fatal("拿 cookie 里的原文去查库了")
	}
	if f.store.lookups[0] != auth.TokenHash(token) {
		t.Fatalf("查库用的不是令牌哈希：%q", f.store.lookups[0])
	}
}

func TestRequireAuthReadsItsOwnCookieByNameOnly(t *testing.T) {
	f := newAuthFixture(false)
	token := f.startSession(t, "u1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/session", nil)
	req.Header.Set("Cookie", "theme=dark; axh_sid="+token+"; locale=zh")
	rec := httptest.NewRecorder()
	f.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("夹在其它 cookie 中间也应认得出来，实际 %d", rec.Code)
	}
}

// 四种失败必须长得一模一样：状态码、Content-Type、响应体、以及清除 cookie 的指令，
// 少一样就够探测者区分「这个令牌不存在」和「这个令牌存在但过期了」。
func TestRequireAuthRejectionsAreIndistinguishable(t *testing.T) {
	prepares := map[string]func(*testing.T, *authFixture) string{
		"令牌查不到": func(_ *testing.T, _ *authFixture) string {
			return "forged-token"
		},
		"会话已过期": func(t *testing.T, f *authFixture) string {
			token := f.startSession(t, "u1")
			hash := auth.TokenHash(token)
			session := f.store.sessions[hash]
			session.ExpiresAt = time.Now().Add(-time.Minute)
			f.store.sessions[hash] = session
			return token
		},
		"用户已被删除": func(t *testing.T, f *authFixture) string {
			return f.startSession(t, "ghost")
		},
		"账号被停用": func(t *testing.T, f *authFixture) string {
			f.store.users["u1"] = model.User{ID: "u1", Email: "anxin@example.com", Role: model.RoleMember, Status: model.UserStatusDisabled}
			return f.startSession(t, "u1")
		},
	}

	type response struct {
		status      int
		contentType string
		body        string
		setCookie   string
	}

	observed := map[string]response{}
	for name, prepare := range prepares {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(false)
			token := prepare(t, f)
			rec := send(f.router(), call{method: http.MethodGet, target: "/api/v1/account/session", cookie: token})
			observed[name] = response{
				status:      rec.Code,
				contentType: rec.Header().Get("Content-Type"),
				body:        rec.Body.String(),
				setCookie:   rec.Header().Get("Set-Cookie"),
			}
		})
	}

	var reference response
	referenceName := ""
	for name, got := range observed {
		if got.status != http.StatusUnauthorized {
			t.Errorf("%s：应返回 401，实际 %d", name, got.status)
		}
		if got.setCookie == "" {
			t.Errorf("%s：应清掉浏览器手里的死 cookie", name)
		}
		if referenceName == "" {
			reference, referenceName = got, name
			continue
		}
		if got != reference {
			t.Errorf("%s 与 %s 的响应可被区分：\n%+v\n%+v", name, referenceName, got, reference)
		}
	}
	if !strings.Contains(reference.body, dto.CodeUnauthorized) {
		t.Errorf("响应体里应是 %s：%s", dto.CodeUnauthorized, reference.body)
	}
}

// 没带 cookie 时不该顺手发一条清除指令——那会给出「你带了个令牌」这个信息，
// 而请求里根本没有令牌。
func TestRequireAuthWithoutCookieSendsNoClearInstruction(t *testing.T) {
	f := newAuthFixture(false)
	rec := send(f.router(), call{method: http.MethodGet, target: "/api/v1/account/session"})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("应返回 401，实际 %d", rec.Code)
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("没有令牌可清，不该发 Set-Cookie：%s", cookie)
	}
	if code := errorCode(t, rec); code != dto.CodeUnauthorized {
		t.Errorf("错误码应为 %s，实际 %s", dto.CodeUnauthorized, code)
	}
}

func TestRequireAuthTurnsStoreFailureIntoInternalError(t *testing.T) {
	f := newAuthFixture(false)
	token := f.startSession(t, "u1")
	f.store.failWith = errors.New("数据库连接失败")

	rec := send(f.router(), call{method: http.MethodGet, target: "/api/v1/account/session", cookie: token})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("仓储故障应是 500，实际 %d", rec.Code)
	}
	if code := errorCode(t, rec); code != dto.CodeInternalError {
		t.Errorf("错误码应为 %s，实际 %s", dto.CodeInternalError, code)
	}
}

func TestRequireRoleAdmitsOnlyItsOwnRole(t *testing.T) {
	f := newAuthFixture(false)
	member := f.startSession(t, "u1")
	admin := f.startSession(t, "admin")

	rec := send(f.router(), call{method: http.MethodPost, target: "/api/v1/admin/articles", cookie: member})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member 访问管理员路由应是 403，实际 %d", rec.Code)
	}
	if code := errorCode(t, rec); code != dto.CodeForbidden {
		t.Errorf("错误码应为 %s，实际 %s", dto.CodeForbidden, code)
	}

	if rec := send(f.router(), call{method: http.MethodPost, target: "/api/v1/admin/articles", cookie: admin}); rec.Code != http.StatusOK {
		t.Errorf("admin 应通过，实际 %d", rec.Code)
	}

	if rec := send(f.router(), call{method: http.MethodPost, target: "/api/v1/admin/articles"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("没登录时应由 RequireAuth 拦下，实际 %d", rec.Code)
	}
}

// 网关层的检查比查会话表便宜得多，必须排在前面：被它挡下的请求不该碰数据库。
func TestOriginCheckRunsBeforeSessionLookup(t *testing.T) {
	f := newAuthFixture(false)
	token := f.startSession(t, "admin")
	f.store.lookups = nil

	router := f.routerWithOrigin(NewOriginPolicy(nil))
	rec := send(router, call{
		method:  http.MethodPost,
		target:  siteURL + "/api/v1/admin/articles",
		headers: ajax("https://evil.example"),
		cookie:  token,
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("跨站来源应被网关挡下，实际 %d", rec.Code)
	}
	if len(f.store.lookups) != 0 {
		t.Errorf("被网关挡下的请求不该查会话表，实际查了 %d 次", len(f.store.lookups))
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	cases := []struct {
		name       string
		secure     bool
		wantSecure bool
	}{
		{name: "开发环境（http）不带 Secure", secure: false, wantSecure: false},
		{name: "生产环境带 Secure", secure: true, wantSecure: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(tc.secure)
			rec := send(f.router(), call{method: http.MethodPost, target: "/api/v1/login"})
			value := rec.Header().Get("Set-Cookie")

			for _, want := range []string{"axh_sid=test-token-value", "Path=/", "HttpOnly", "SameSite=Lax", "Max-Age=2592000"} {
				if !strings.Contains(value, want) {
					t.Errorf("cookie 缺少 %q：%s", want, value)
				}
			}
			if strings.Contains(value, "Secure") != tc.wantSecure {
				t.Errorf("Secure 应为 %v：%s", tc.wantSecure, value)
			}
			if strings.Contains(value, "%") {
				t.Errorf("cookie 值被转义了，说明令牌的字符集选错了：%s", value)
			}
		})
	}
}

// Go 的 http.Cookie 在 MaxAge 为负时只写 Max-Age=0，不写 Expires（Expires 只在
// 显式设过时才输出）。Max-Age=0 就是「立刻过期」，不需要再补一个过去的时间点。
func TestClearSessionCookieExpiresImmediately(t *testing.T) {
	f := newAuthFixture(true)
	rec := send(f.router(), call{method: http.MethodPost, target: "/api/v1/logout"})
	value := rec.Header().Get("Set-Cookie")

	if !strings.HasPrefix(value, "axh_sid=;") {
		t.Errorf("应把 cookie 的值清空：%s", value)
	}
	for _, want := range []string{"Max-Age=0", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(value, want) {
			t.Errorf("清除用的 cookie 缺少 %q：%s", want, value)
		}
	}
}
