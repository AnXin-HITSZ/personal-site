package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/service"
)

// 这几个接口的请求体只有邮箱和口令，4KB 绰绰有余。
// 不设上限的话，一个几 GB 的 body 就能把内存吃干。
const maxRequestBodyBytes = 4 << 10

// 一篇文章的正文比邮箱口令大两个数量级，沿用上面那个上限会把正常的长文截掉。
// 它必须小于 nginx 的 client_max_body_size，否则请求根本到不了这里。
const maxArticleJSONBytes = 2 << 20

// 这一层只声明限速器「能回答什么」，不声明它是怎么算的——所以 handler 不认识
// ratelimit 包，换一个实现（进程内换成共享存储、固定窗口换成令牌桶）不必动
// 这一层的任何决策代码：用哪个 key、什么顺序、什么时候清，都留在下面。
type rateLimiter interface {
	Allow(key string) (bool, time.Duration)
	Reset(key string)
}

// 限速器由外面传进来，是为了测试能塞一个 limit=1 的进去——
// 策略写在 main.go 里，看得见每一条是几分钟几次。
type AccountLimits struct {
	RegisterPerIP    rateLimiter
	LoginPerIP       rateLimiter
	LoginPerEmail    rateLimiter
	PasswordPerIP    rateLimiter
	PasswordPerEmail rateLimiter
	ResendPerEmail   rateLimiter
}

// 和 service 里那几个 Store 接口同一个理由：把依赖收窄成接口，这一层的测试
// 才只测 HTTP——状态码、响应体、cookie、限速顺序——不必再拖一个假数据库进来。
type accountService interface {
	Register(ctx context.Context, email, password string) error
	VerifyEmail(ctx context.Context, token string) error
	ResendVerification(ctx context.Context, email string) error
	Login(ctx context.Context, email, password, userAgent, ip string) (*service.LoginResult, error)
	Logout(ctx context.Context, token string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, password string) error
	ChangePassword(ctx context.Context, userID, keepSessionID, currentPassword, nextPassword string) (int64, error)
	ListSessions(ctx context.Context, userID string) ([]model.Session, error)
	RevokeSession(ctx context.Context, userID, sessionID string) (int64, error)
}

type Account struct {
	accounts accountService
	cookies  middleware.SessionCookie
	limits   AccountLimits
}

func NewAccount(accounts accountService, cookies middleware.SessionCookie, limits AccountLimits) *Account {
	return &Account{accounts: accounts, cookies: cookies, limits: limits}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	Password        string `json:"password"`
}

// 成功一律是 202 加同一句话，无论邮箱是否已经被占用、是否真的存在。
// 这两个接口的作用是「请给我发一封信」，而信发不发得出去，不该由响应体说明。
func (h *Account) Register(c *gin.Context) {
	if h.tooMany(c, h.limits.RegisterPerIP, c.ClientIP(), "注册太频繁，请稍后再试") {
		return
	}

	var request registerRequest
	if !bindJSON(c, &request) {
		return
	}

	err := h.accounts.Register(c.Request.Context(), request.Email, request.Password)
	if respondUserInputError(c, err) {
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusAccepted, dto.NewMessage("如果这个邮箱可以注册，验证邮件已经发出"))
}

func (h *Account) VerifyEmail(c *gin.Context) {
	var request tokenRequest
	if !bindJSON(c, &request) {
		return
	}

	err := h.accounts.VerifyEmail(c.Request.Context(), request.Token)
	if errors.Is(err, service.ErrTokenInvalid) {
		c.JSON(http.StatusBadRequest, dto.NewInvalidToken())
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, dto.NewMessage("邮箱验证完成"))
}

func (h *Account) ResendVerification(c *gin.Context) {
	var request emailRequest
	if !bindJSON(c, &request) {
		return
	}
	if h.tooMany(c, h.limits.ResendPerEmail, mailKey(request.Email), "验证邮件发得太频繁，请稍后再试") {
		return
	}

	err := h.accounts.ResendVerification(c.Request.Context(), request.Email)
	if respondUserInputError(c, err) {
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusAccepted, dto.NewMessage("如果这个邮箱需要验证，验证邮件已经发出"))
}

func (h *Account) Login(c *gin.Context) {
	if h.tooMany(c, h.limits.LoginPerIP, c.ClientIP(), "登录尝试过于频繁，请稍后再试") {
		return
	}

	var request loginRequest
	if !bindJSON(c, &request) {
		return
	}

	// 两维都要限：只按 IP 限，换一批 IP 就能继续爆同一个账号；只按邮箱限，
	// 同一批口令就能拿去撞很多账号。邮箱这一维的 key 用归一化前的粗处理就够了——
	// 它只是个计数器的名字，不需要是合法的邮箱。
	emailKey := mailKey(request.Email)
	if h.tooMany(c, h.limits.LoginPerEmail, emailKey, "这个邮箱的登录尝试过于频繁，请稍后再试") {
		return
	}

	result, err := h.accounts.Login(c.Request.Context(), request.Email, request.Password, c.Request.UserAgent(), c.ClientIP())
	switch {
	case errors.Is(err, service.ErrCredentialsInvalid):
		c.JSON(http.StatusUnauthorized, dto.NewInvalidCredentials())
		return
	case errors.Is(err, service.ErrEmailNotVerified):
		c.JSON(http.StatusForbidden, dto.NewEmailNotVerified())
		return
	case errors.Is(err, service.ErrAccountDisabled):
		c.JSON(http.StatusForbidden, dto.NewAccountDisabled())
		return
	case err != nil:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	// 只清「按邮箱」那一维。IP 那一维绝不能清：清掉它，攻击者只要手上有一个
	// 能登录的账号，就能靠它反复把自己的 IP 计数归零，继续爆破别人。
	h.limits.LoginPerEmail.Reset(emailKey)
	h.cookies.Set(c, result.Token)
	c.JSON(http.StatusOK, dto.NewSessionView(result.User, result.ExpiresAt))
}

// 不需要 RequireAuth：cookie 已经过期的用户也得能点「退出」，
// 而且这个接口本来就只对「删掉自己手上这个令牌」负责。
func (h *Account) Logout(c *gin.Context) {
	if err := h.accounts.Logout(c.Request.Context(), h.cookies.Read(c)); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	h.cookies.Clear(c)
	c.Status(http.StatusNoContent)
}

func (h *Account) Session(c *gin.Context) {
	account, ok := middleware.CurrentAccount(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.NewUnauthorized())
		return
	}
	c.JSON(http.StatusOK, dto.NewSessionView(account.User, account.Session.ExpiresAt))
}

func (h *Account) ForgotPassword(c *gin.Context) {
	if h.tooMany(c, h.limits.PasswordPerIP, c.ClientIP(), "请求太频繁，请稍后再试") {
		return
	}

	var request emailRequest
	if !bindJSON(c, &request) {
		return
	}
	if h.tooMany(c, h.limits.PasswordPerEmail, mailKey(request.Email), "该邮箱的请求太频繁，请稍后再试") {
		return
	}

	err := h.accounts.RequestPasswordReset(c.Request.Context(), request.Email)
	if respondUserInputError(c, err) {
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusAccepted, dto.NewMessage("如果这个邮箱有账号，重置口令的邮件已经发出"))
}

func (h *Account) ResetPassword(c *gin.Context) {
	var request resetPasswordRequest
	if !bindJSON(c, &request) {
		return
	}

	err := h.accounts.ResetPassword(c.Request.Context(), request.Token, request.Password)
	if errors.Is(err, service.ErrTokenInvalid) {
		c.JSON(http.StatusBadRequest, dto.NewInvalidToken())
		return
	}
	if respondUserInputError(c, err) {
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	// 重置口令会把所有会话都删掉，这个浏览器手上那个也一样，顺手清掉，
	// 免得它继续带着一个已经失效的令牌。
	h.cookies.Clear(c)
	c.Status(http.StatusNoContent)
}

func (h *Account) ChangePassword(c *gin.Context) {
	account, ok := middleware.CurrentAccount(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.NewUnauthorized())
		return
	}

	var request changePasswordRequest
	if !bindJSON(c, &request) {
		return
	}

	kicked, err := h.accounts.ChangePassword(c.Request.Context(),
		account.User.ID, account.Session.ID, request.CurrentPassword, request.Password)
	switch {
	case errors.Is(err, service.ErrCurrentPasswordWrong):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("currentPassword", err.Error()))
		return
	case errors.Is(err, service.ErrPasswordUnchanged):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("password", err.Error()))
		return
	}
	if respondUserInputError(c, err) {
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, dto.NewMessage(kickedMessage(kicked)))
}

func (h *Account) ListSessions(c *gin.Context) {
	account, ok := middleware.CurrentAccount(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.NewUnauthorized())
		return
	}

	sessions, err := h.accounts.ListSessions(c.Request.Context(), account.User.ID)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	view := dto.DeviceListView{Sessions: make([]dto.DeviceView, 0, len(sessions))}
	for _, session := range sessions {
		view.Sessions = append(view.Sessions, dto.NewDeviceView(session, session.ID == account.Session.ID))
	}
	c.JSON(http.StatusOK, view)
}

func (h *Account) RevokeSession(c *gin.Context) {
	account, ok := middleware.CurrentAccount(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.NewUnauthorized())
		return
	}

	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("id", "会话标识不合法"))
		return
	}
	// 删掉当前这一条等于退出登录，那是另一个接口的事。这里挡下来，
	// 免得前端在设备列表里点自己那台，得到一个「已登出」的意外结果。
	if sessionID == account.Session.ID {
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("id", "不能在这里删除当前设备，请使用退出登录"))
		return
	}

	deleted, err := h.accounts.RevokeSession(c.Request.Context(), account.User.ID, sessionID)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}
	// 0 行表示「不存在」或者「不属于你」，这两件事不区分。
	if deleted == 0 {
		c.JSON(http.StatusNotFound, dto.NewNotFound("会话不存在"))
		return
	}

	c.Status(http.StatusNoContent)
}

func kickedMessage(kicked int64) string {
	if kicked == 0 {
		return "口令已更新"
	}
	return "口令已更新，其它 " + strconv.FormatInt(kicked, 10) + " 台设备已登出"
}

// 维度由限速器的名字承担，这里只负责把值归一化：大小写和首尾空格
// 都不该算成另一个邮箱。
func mailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (h *Account) tooMany(c *gin.Context, limiter rateLimiter, key, message string) bool {
	allowed, retryAfter := limiter.Allow(key)
	if allowed {
		return false
	}

	seconds := int(retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.JSON(http.StatusTooManyRequests, dto.NewTooManyRequests(message))
	return true
}

// 截断请求体，免得一个超大 body 把内存吃干。
func bindJSON(c *gin.Context, target any) bool {
	return bindJSONLimit(c, target, maxRequestBodyBytes)
}

// 截断和 JSON 语法错在 ShouldBindJSON 眼里都是同一个失败。回一句「请求格式不正确」，
// 会让人对着明明合法的 JSON 找半天的格式问题——所以超限要单独认出来说清楚。
func bindJSONLimit(c *gin.Context, target any, limit int64) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)

	err := c.ShouldBindJSON(target)
	if err == nil {
		return true
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, dto.NewPayloadTooLarge("请求体过大"))
		return false
	}

	c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("body", "请求格式不正确"))
	return false
}

// 只认这几种错误是用户输入的问题。其余一律当服务端故障——把数据库错误
// 当成 400 回给用户，等于让人以为是自己填错了。返回 true 表示已经回过响应了。
func respondUserInputError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, auth.ErrEmailInvalid):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("email", auth.ErrEmailInvalid.Error()))
	case errors.Is(err, auth.ErrPasswordTooShort),
		errors.Is(err, auth.ErrPasswordTooLong),
		errors.Is(err, auth.ErrPasswordSameMail):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("password", err.Error()))
	default:
		return false
	}
	return true
}
