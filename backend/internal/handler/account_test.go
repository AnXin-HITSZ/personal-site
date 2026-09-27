package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/ratelimit"
	"anxin-hitsz.com/backend/internal/repository"
	"anxin-hitsz.com/backend/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// 每个方法都是一个可替换的函数字段：用例只写下自己关心的那一个，
// 其余留空。这样这个假实现不用为「测试要断言什么」预先长出几十个字段。
type fakeAccountService struct {
	register     func(email, password string) error
	verifyEmail  func(token string) error
	resend       func(email string) error
	login        func(email, password, userAgent, ip string) (*service.LoginResult, error)
	logout       func(token string) error
	forgot       func(email string) error
	reset        func(token, password string) error
	change       func(userID, keepSessionID, current, next string) (int64, error)
	listSessions func(userID string) ([]model.Session, error)
	revoke       func(userID, sessionID string) (int64, error)
}

func (f *fakeAccountService) Register(_ context.Context, email, password string) error {
	if f.register == nil {
		return nil
	}
	return f.register(email, password)
}

func (f *fakeAccountService) VerifyEmail(_ context.Context, token string) error {
	if f.verifyEmail == nil {
		return nil
	}
	return f.verifyEmail(token)
}

func (f *fakeAccountService) ResendVerification(_ context.Context, email string) error {
	if f.resend == nil {
		return nil
	}
	return f.resend(email)
}

// 默认返回「凭据无效」而不是空结果：留空时若返回 (nil, nil)，
// handler 会去解引用一个 nil，测试报的是 panic 而不是它想说的那件事。
func (f *fakeAccountService) Login(_ context.Context, email, password, userAgent, ip string) (*service.LoginResult, error) {
	if f.login == nil {
		return nil, service.ErrCredentialsInvalid
	}
	return f.login(email, password, userAgent, ip)
}

func (f *fakeAccountService) Logout(_ context.Context, token string) error {
	if f.logout == nil {
		return nil
	}
	return f.logout(token)
}

func (f *fakeAccountService) RequestPasswordReset(_ context.Context, email string) error {
	if f.forgot == nil {
		return nil
	}
	return f.forgot(email)
}

func (f *fakeAccountService) ResetPassword(_ context.Context, token, password string) error {
	if f.reset == nil {
		return nil
	}
	return f.reset(token, password)
}

func (f *fakeAccountService) ChangePassword(_ context.Context, userID, keepSessionID, current, next string) (int64, error) {
	if f.change == nil {
		return 0, nil
	}
	return f.change(userID, keepSessionID, current, next)
}

func (f *fakeAccountService) ListSessions(_ context.Context, userID string) ([]model.Session, error) {
	if f.listSessions == nil {
		return nil, nil
	}
	return f.listSessions(userID)
}

func (f *fakeAccountService) RevokeSession(_ context.Context, userID, sessionID string) (int64, error) {
	if f.revoke == nil {
		return 0, nil
	}
	return f.revoke(userID, sessionID)
}

// 只实现 SessionStore 的四个方法。RequireAuth 是真的，走的就是生产那条路径——
// 把中间件换掉再测 handler，测的就不是同一件事了。
type fakeSessionStore struct {
	user    *model.User
	session *model.Session
}

func (s *fakeSessionStore) CreateSession(_ context.Context, session *model.Session) error {
	stored := *session
	s.session = &stored
	return nil
}

func (s *fakeSessionStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*model.Session, error) {
	if s.session == nil || s.session.TokenHash != tokenHash {
		return nil, repository.ErrNotFound
	}
	return s.session, nil
}

func (s *fakeSessionStore) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, repository.ErrNotFound
	}
	return s.user, nil
}

func (s *fakeSessionStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	if s.session != nil && s.session.TokenHash == tokenHash {
		s.session = nil
	}
	return nil
}

const testUserAgent = "Mozilla/5.0 (Test Browser)"

type handlerFixture struct {
	svc      *fakeAccountService
	store    *fakeSessionStore
	sessions *service.Session
	cookies  middleware.SessionCookie
	router   *gin.Engine
}

// 默认额度给得很宽：绝大多数用例关心的是状态码和响应体，不是限速。
// 要测限速的用例自己传一份很紧的进来。
func generousLimits() AccountLimits {
	return AccountLimits{
		RegisterPerIP:    ratelimit.New(1000, time.Hour),
		LoginPerIP:       ratelimit.New(1000, time.Hour),
		LoginPerEmail:    ratelimit.New(1000, time.Hour),
		PasswordPerIP:    ratelimit.New(1000, time.Hour),
		PasswordPerEmail: ratelimit.New(1000, time.Hour),
		ResendPerEmail:   ratelimit.New(1000, time.Hour),
	}
}

func newHandlerFixture(limits AccountLimits) *handlerFixture {
	store := &fakeSessionStore{user: &model.User{
		ID:        "u1",
		Email:     "owner@example.com",
		Role:      model.RoleMember,
		Status:    model.UserStatusActive,
		CreatedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}}
	sessions := service.NewSession(store)
	cookies := middleware.NewSessionCookie(false)
	account := NewAccount(&fakeAccountService{}, cookies, limits)

	router := gin.New()
	router.Use(middleware.LogServerErrors(), middleware.Recovery())

	api := router.Group("/api/v1")
	auth := api.Group("/auth")
	auth.POST("/register", account.Register)
	auth.POST("/verify-email", account.VerifyEmail)
	auth.POST("/resend-verification", account.ResendVerification)
	auth.POST("/login", account.Login)
	auth.POST("/logout", account.Logout)
	auth.POST("/forgot-password", account.ForgotPassword)
	auth.POST("/reset-password", account.ResetPassword)

	me := api.Group("/account", middleware.RequireAuth(cookies, sessions))
	me.GET("/session", account.Session)
	me.POST("/password", account.ChangePassword)
	me.GET("/sessions", account.ListSessions)
	me.DELETE("/sessions/:id", account.RevokeSession)

	return &handlerFixture{svc: account.accounts.(*fakeAccountService), store: store,
		sessions: sessions, cookies: cookies, router: router}
}

func (f *handlerFixture) post(path, body, cookie string) *httptest.ResponseRecorder {
	return f.request(http.MethodPost, path, body, cookie)
}

func (f *handlerFixture) request(method, path, body, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://anxin-hitsz.com"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", testUserAgent)
	if cookie != "" {
		req.Header.Set("Cookie", "axh_sid="+cookie)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// 新建一个真实登录态的会话，返回令牌。RequireAuth 走的是生产代码，
// 所以这里必须真的有一条能查到的会话。
func (f *handlerFixture) startSession(t *testing.T) string {
	t.Helper()
	token, _, err := f.sessions.Start(context.Background(), f.store.user.ID, "Mozilla/5.0", "203.0.113.7")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	return token
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body dto.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	return body.Error.Code
}

func errorField(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body dto.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	return body.Error.Field
}

func setCookie(rec *httptest.ResponseRecorder) string {
	return rec.Header().Get("Set-Cookie")
}

func loginBody(email, password string) string {
	return `{"email":"` + email + `","password":"` + password + `"}`
}

// 注册成功一律 202 加同一句话。响应体里不能出现邮箱，也不能出现任何
// 让人能推断出「这个邮箱是不是已经被占了」的东西。
func TestRegisterAcceptedShape(t *testing.T) {
	f := newHandlerFixture(generousLimits())

	rec := f.post("/api/v1/auth/register", loginBody("owner@example.com", "a-long-enough-password"), "")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("应为 202，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "owner@example.com") {
		t.Errorf("响应体不该带邮箱：%s", rec.Body.String())
	}
	if setCookie(rec) != "" {
		t.Errorf("注册不该设置 cookie：%s", setCookie(rec))
	}
}

// 校验失败是用户自己的问题，要指到具体哪个字段。
func TestRegisterInputErrorsNameTheField(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantField  string
	}{
		{name: "邮箱格式不对", serviceErr: auth.ErrEmailInvalid, wantField: "email"},
		{name: "口令过短", serviceErr: auth.ErrPasswordTooShort, wantField: "password"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerFixture(generousLimits())
			f.svc.register = func(string, string) error { return tc.serviceErr }

			rec := f.post("/api/v1/auth/register", loginBody("owner@example.com", "whatever-long"), "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("应为 400，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != dto.CodeInvalidArgument {
				t.Errorf("应为 %s，实际 %s", dto.CodeInvalidArgument, code)
			}
			if field := errorField(t, rec); field != tc.wantField {
				t.Errorf("字段应为 %s，实际 %s", tc.wantField, field)
			}
		})
	}
}

// 数据库出错被回成 400，用户会以为是自己填错了，然后一直改。必须是 500。
func TestRegisterTreatsServiceFailureAsServerError(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	f.svc.register = func(string, string) error { return errors.New("Error 1146: Table 'users' doesn't exist") }

	rec := f.post("/api/v1/auth/register", loginBody("owner@example.com", "whatever-long"), "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("应为 500，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != dto.CodeInternalError {
		t.Errorf("应为 %s，实际 %s", dto.CodeInternalError, code)
	}
	if strings.Contains(rec.Body.String(), "1146") {
		t.Error("响应体不该出现内部错误细节")
	}
}

func TestLoginSetsCookieAndReturnsAccount(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	f.svc.login = func(email, password, userAgent, ip string) (*service.LoginResult, error) {
		if email != "owner@example.com" || password != "a-long-enough-password" {
			t.Errorf("传给 service 的参数不对：%s / %s", email, password)
		}
		if ip != "192.0.2.1" {
			t.Errorf("客户端 IP 应传下去，实际 %q", ip)
		}
		if userAgent != testUserAgent {
			t.Errorf("User-Agent 应传下去，实际 %q", userAgent)
		}
		return &service.LoginResult{
			User:      *f.store.user,
			Token:     "session-token-value",
			ExpiresAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		}, nil
	}

	rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), "")

	if rec.Code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d：%s", rec.Code, rec.Body.String())
	}

	cookie := setCookie(rec)
	for _, want := range []string{"axh_sid=session-token-value", "Path=/", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("cookie 缺少 %s：%s", want, cookie)
		}
	}
	if strings.Contains(cookie, "%") {
		t.Errorf("cookie 值被转义了，说明令牌字母表不合规：%s", cookie)
	}

	body := rec.Body.String()
	for _, want := range []string{`"email":"owner@example.com"`, `"role":"member"`, `"expiresAt"`} {
		if !strings.Contains(body, want) {
			t.Errorf("响应体缺少 %s：%s", want, body)
		}
	}
	for _, forbidden := range []string{"passwordHash", "password_hash", "$2a$"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("响应体泄露了 %s：%s", forbidden, body)
		}
	}
}

func TestLoginErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{name: "凭据不对", serviceErr: service.ErrCredentialsInvalid, wantStatus: http.StatusUnauthorized, wantCode: dto.CodeInvalidCredentials},
		{name: "邮箱未验证", serviceErr: service.ErrEmailNotVerified, wantStatus: http.StatusForbidden, wantCode: dto.CodeEmailNotVerified},
		{name: "账号停用", serviceErr: service.ErrAccountDisabled, wantStatus: http.StatusForbidden, wantCode: dto.CodeAccountDisabled},
		{name: "数据库故障", serviceErr: errors.New("Error 1146"), wantStatus: http.StatusInternalServerError, wantCode: dto.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerFixture(generousLimits())
			f.svc.login = func(string, string, string, string) (*service.LoginResult, error) {
				return nil, tc.serviceErr
			}

			rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), "")

			if rec.Code != tc.wantStatus {
				t.Fatalf("应为 %d，实际 %d：%s", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != tc.wantCode {
				t.Errorf("应为 %s，实际 %s", tc.wantCode, code)
			}
			// 失败时绝不能顺手给个 cookie，否则浏览器会带着一个没用的令牌到处跑。
			if setCookie(rec) != "" {
				t.Errorf("登录失败不该设置 cookie：%s", setCookie(rec))
			}
		})
	}
}

func TestLoginRateLimitByAddress(t *testing.T) {
	limits := generousLimits()
	limits.LoginPerIP = ratelimit.New(2, 15*time.Minute)

	f := newHandlerFixture(limits)
	calls := 0
	f.svc.login = func(string, string, string, string) (*service.LoginResult, error) {
		calls++
		return nil, service.ErrCredentialsInvalid
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次应为 401，实际 %d", attempt, rec.Code)
		}
	}

	rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), "")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("第三次应为 429，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != dto.CodeTooManyRequests {
		t.Errorf("应为 %s，实际 %s", dto.CodeTooManyRequests, code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 应带 Retry-After")
	}
	if calls != 2 {
		t.Errorf("被限速之后不该再调用 service，实际调用 %d 次", calls)
	}
}

// 只按 IP 限挡不住换 IP 撞同一个账号，所以还要按邮箱限一维。
func TestLoginRateLimitByEmail(t *testing.T) {
	limits := generousLimits()
	limits.LoginPerEmail = ratelimit.New(1, 15*time.Minute)

	f := newHandlerFixture(limits)

	if rec := f.post("/api/v1/auth/login", loginBody("victim@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("第一次应为 401，实际 %d", rec.Code)
	}
	if rec := f.post("/api/v1/auth/login", loginBody("victim@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("同一个邮箱第二次应为 429，实际 %d", rec.Code)
	}
	// 换个邮箱不该被牵连。
	if rec := f.post("/api/v1/auth/login", loginBody("someone@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("换邮箱不该被牵连，实际 %d", rec.Code)
	}
}

// 登录成功只清「按邮箱」那一维。IP 那一维要是也清了，攻击者只要手上有一个
// 能登录的账号，就能靠它反复把自己的 IP 计数归零，继续爆破别人。
func TestLoginSuccessResetsEmailLimitButNotAddressLimit(t *testing.T) {
	limits := generousLimits()
	limits.LoginPerIP = ratelimit.New(3, 15*time.Minute)
	limits.LoginPerEmail = ratelimit.New(2, 15*time.Minute)

	f := newHandlerFixture(limits)
	attempt := 0
	f.svc.login = func(string, string, string, string) (*service.LoginResult, error) {
		attempt++
		// 第二次给它成功，其余失败。
		if attempt == 2 {
			return &service.LoginResult{User: *f.store.user, Token: "t", ExpiresAt: time.Now().Add(time.Hour)}, nil
		}
		return nil, service.ErrCredentialsInvalid
	}

	if rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("第一次应 401，实际 %d", rec.Code)
	}
	if rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusOK {
		t.Fatalf("第二次应成功，实际 %d", rec.Code)
	}
	// 邮箱这一维的计数已经被成功那次清掉了，所以第三次能走到 service。
	// 没清的话，第二次就已经把邮箱额度用满，第三次会是 429。
	if rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("第三次应为 401（邮箱计数已被成功那次重置），实际 %d", rec.Code)
	}
	// IP 这一维的计数没被清：三次已经用满，第四次必须被拦。
	if rec := f.post("/api/v1/auth/login", loginBody("owner@example.com", "a-long-enough-password"), ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("第四次应为 429（IP 计数不该被成功那次重置），实际 %d", rec.Code)
	}
}

// 退出不需要登录态：cookie 已经过期的用户也得点得动这个按钮。
func TestLogoutClearsCookieWithoutRequiringLogin(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)
	f.svc.logout = func(got string) error {
		if got != token {
			t.Errorf("应把手上的令牌交给 service，实际 %q", got)
		}
		return nil
	}

	rec := f.post("/api/v1/auth/logout", "", token)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应为 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	cookie := setCookie(rec)
	if !strings.Contains(cookie, "axh_sid=;") || !strings.Contains(cookie, "Max-Age=0") {
		t.Errorf("应清掉 cookie，实际 %s", cookie)
	}
}

func TestForgotPasswordIsAlwaysAccepted(t *testing.T) {
	f := newHandlerFixture(generousLimits())

	rec := f.post("/api/v1/auth/forgot-password", `{"email":"nobody@example.com"}`, "")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("应为 202，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if setCookie(rec) != "" {
		t.Error("这个接口不该设置 cookie")
	}
}

func TestResetPasswordClearsCookie(t *testing.T) {
	f := newHandlerFixture(generousLimits())

	rec := f.post("/api/v1/auth/reset-password", `{"token":"t","password":"a-long-enough-password"}`, "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应为 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	// 重置会把所有会话删掉，这个浏览器手上那个也没用了。
	if cookie := setCookie(rec); !strings.Contains(cookie, "Max-Age=0") {
		t.Errorf("应清掉 cookie，实际 %s", cookie)
	}
}

func TestResetPasswordRejectsSpentToken(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	f.svc.reset = func(string, string) error { return service.ErrTokenInvalid }

	rec := f.post("/api/v1/auth/reset-password", `{"token":"spent","password":"a-long-enough-password"}`, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应为 400，实际 %d", rec.Code)
	}
	if code := errorCode(t, rec); code != dto.CodeInvalidToken {
		t.Errorf("应为 %s，实际 %s", dto.CodeInvalidToken, code)
	}
}

func TestAuthenticatedRoutesRequireLogin(t *testing.T) {
	f := newHandlerFixture(generousLimits())

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/account/session"},
		{method: http.MethodPost, path: "/api/v1/account/password", body: `{"currentPassword":"a","password":"b"}`},
		{method: http.MethodGet, path: "/api/v1/account/sessions"},
		{method: http.MethodDelete, path: "/api/v1/account/sessions/s9"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := f.request(tc.method, tc.path, tc.body, "")

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("应 401，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != dto.CodeUnauthorized {
				t.Errorf("应为 %s，实际 %s", dto.CodeUnauthorized, code)
			}
		})
	}
}

func TestSessionViewReportsCurrentAccount(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)

	rec := f.request(http.MethodGet, "/api/v1/account/session", "", token)

	if rec.Code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"email":"owner@example.com"`) {
		t.Errorf("应返回当前账号：%s", body)
	}
	if strings.Contains(body, "$2a$") {
		t.Errorf("泄露了口令哈希：%s", body)
	}
}

func TestChangePasswordMapsErrorsToFields(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantField  string
	}{
		{name: "当前口令不对", serviceErr: service.ErrCurrentPasswordWrong, wantField: "currentPassword"},
		{name: "新口令和当前相同", serviceErr: service.ErrPasswordUnchanged, wantField: "password"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerFixture(generousLimits())
			token := f.startSession(t)
			f.svc.change = func(string, string, string, string) (int64, error) { return 0, tc.serviceErr }

			rec := f.request(http.MethodPost, "/api/v1/account/password",
				`{"currentPassword":"old-long-password","password":"new-long-password"}`, token)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("应为 400，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if field := errorField(t, rec); field != tc.wantField {
				t.Errorf("字段应为 %s，实际 %s", tc.wantField, field)
			}
		})
	}
}

func TestChangePasswordKeepsCurrentSessionID(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)

	f.svc.change = func(userID, keepSessionID, current, next string) (int64, error) {
		if userID != "u1" {
			t.Errorf("应传当前账号 ID，实际 %q", userID)
		}
		// 「当前会话」必须由服务端从 cookie 里解析出来，不能由前端说了算。
		if keepSessionID != f.store.session.ID {
			t.Errorf("应保留当前会话 %q，实际 %q", f.store.session.ID, keepSessionID)
		}
		return 2, nil
	}

	rec := f.request(http.MethodPost, "/api/v1/account/password",
		`{"currentPassword":"old-long-password","password":"new-long-password"}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "2") {
		t.Errorf("应说明踢掉了几台设备：%s", rec.Body.String())
	}
}

func TestRevokeSessionRefusesTheCurrentDevice(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)

	called := false
	f.svc.revoke = func(string, string) (int64, error) {
		called = true
		return 1, nil
	}

	rec := f.request(http.MethodDelete, "/api/v1/account/sessions/"+f.store.session.ID, "", token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应为 400，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Error("当前设备不该走到删除逻辑——那是退出登录该做的事")
	}
}

func TestRevokeSessionUnknownIDIsNotFound(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)
	f.svc.revoke = func(userID, sessionID string) (int64, error) {
		if userID != "u1" || sessionID != "s9" {
			t.Errorf("定位参数不对：%s / %s", userID, sessionID)
		}
		return 0, nil
	}

	rec := f.request(http.MethodDelete, "/api/v1/account/sessions/s9", "", token)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

func TestListSessionsMarksTheCurrentDevice(t *testing.T) {
	f := newHandlerFixture(generousLimits())
	token := f.startSession(t)
	current := f.store.session.ID

	f.svc.listSessions = func(string) ([]model.Session, error) {
		return []model.Session{
			{ID: current, UserID: "u1", TokenHash: strings.Repeat("a", 64), UserAgent: "当前这台", IP: "203.0.113.7"},
			{ID: "s2", UserID: "u1", TokenHash: strings.Repeat("b", 64), UserAgent: "别处那台", IP: "198.51.100.9"},
		}, nil
	}

	rec := f.request(http.MethodGet, "/api/v1/account/sessions", "", token)

	if rec.Code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d：%s", rec.Code, rec.Body.String())
	}

	var body dto.DeviceListView
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是设备列表：%s", rec.Body.String())
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("应有两台设备，实际 %d 台", len(body.Sessions))
	}
	for _, device := range body.Sessions {
		want := device.ID == current
		if device.Current != want {
			t.Errorf("设备 %s 的 current 应为 %v", device.ID, want)
		}
	}
	if strings.Contains(rec.Body.String(), "tokenHash") || strings.Contains(rec.Body.String(), strings.Repeat("a", 64)) {
		t.Error("设备列表不该暴露令牌哈希")
	}
}

func TestMalformedRequestsAreRejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "不是 JSON", body: "这不是 JSON"},
		{name: "空 body", body: ""},
		{name: "超大 body", body: `{"email":"` + strings.Repeat("a", 5<<10) + `"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerFixture(generousLimits())
			called := false
			f.svc.register = func(string, string) error { called = true; return nil }

			rec := f.post("/api/v1/auth/register", tc.body, "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("应为 400，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if called {
				t.Error("解析都没过就不该调用 service")
			}
		})
	}
}
