package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/mail"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

const (
	EmailVerifyTTL   = 24 * time.Hour
	PasswordResetTTL = 30 * time.Minute
)

var (
	ErrCredentialsInvalid   = errors.New("邮箱或口令不正确")
	ErrEmailNotVerified     = errors.New("邮箱尚未验证")
	ErrAccountDisabled      = errors.New("账号已停用")
	ErrTokenInvalid         = errors.New("链接无效或已过期")
	ErrCurrentPasswordWrong = errors.New("当前口令不正确")
	ErrPasswordUnchanged    = errors.New("新口令和当前口令相同")
)

// 和 SessionStore 同样的理由：仓储是具体类型，这里收窄成接口，
// 账号流程才测得动——注册要发信、要写库，但不需要一个真的 MySQL。
type AccountStore interface {
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	MarkEmailVerified(ctx context.Context, userID string, at time.Time) error
	UpdatePasswordHash(ctx context.Context, userID, hash string) error
	CreateAuthToken(ctx context.Context, token *model.AuthToken) error
	ReplaceAuthToken(ctx context.Context, token *model.AuthToken) error
	ConsumeAuthToken(ctx context.Context, kind, tokenHash string, now time.Time) (*model.AuthToken, error)
	DeleteSessionsByUser(ctx context.Context, userID, exceptID string) (int64, error)
	ListSessionsByUser(ctx context.Context, userID string) ([]model.Session, error)
	DeleteSession(ctx context.Context, userID, sessionID string) (int64, error)
}

type Account struct {
	store    AccountStore
	sessions *Session
	mailer   mail.Mailer
	baseURL  string
	now      func() time.Time
}

func NewAccount(store AccountStore, sessions *Session, mailer mail.Mailer, baseURL string) *Account {
	return &Account{store: store, sessions: sessions, mailer: mailer, baseURL: baseURL, now: time.Now}
}

// 邮箱被占用时也返回 nil。调用者据此写出和成功一模一样的响应，
// 这个接口才不会变成一个「查邮箱注册过没有」的查询接口。
func (a *Account) Register(ctx context.Context, email, password string) error {
	normalized, err := auth.NormalizeEmail(email)
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(password, normalized); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	id, err := auth.NewID()
	if err != nil {
		return err
	}

	now := a.now()
	user := &model.User{
		ID:           id,
		Email:        normalized,
		PasswordHash: hash,
		Role:         model.RoleMember,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = a.store.CreateUser(ctx, user)
	if errors.Is(err, repository.ErrEmailTaken) {
		// 对外不区分，但总得让真正的主人知道有人在试他的邮箱。
		a.send(ctx, normalized, "有人用你的邮箱注册",
			"有人用这个邮箱在本站发起了注册，但账号已经存在，所以什么也没发生。\n\n"+
				"如果那就是你，直接登录即可；忘了口令就用「忘记口令」重设。\n"+
				"如果不是你，这封邮件可以直接忽略。")
		return nil
	}
	if err != nil {
		return err
	}

	token, err := a.issueToken(ctx, user.ID, model.TokenEmailVerify, EmailVerifyTTL, false)
	if err != nil {
		return err
	}
	a.sendVerification(ctx, user.Email, token)
	return nil
}

// 令牌先消费、再改状态：这样即使后面那步失败了，被偷走的链接也已经作废。
// 失败是「需要重新申请一封」，而不是「链接还能被重放」。
func (a *Account) VerifyEmail(ctx context.Context, token string) error {
	consumed, err := a.consumeToken(ctx, model.TokenEmailVerify, token)
	if err != nil {
		return err
	}
	return a.store.MarkEmailVerified(ctx, consumed.UserID, a.now())
}

func (a *Account) ResendVerification(ctx context.Context, email string) error {
	normalized, err := auth.NormalizeEmail(email)
	if err != nil {
		return err
	}

	user, err := a.store.GetUserByEmail(ctx, normalized)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// 已经验证过就什么也不做。可以直接返回，因为响应和「邮箱不存在」完全一样。
	if user.EmailVerifiedAt != nil {
		return nil
	}

	// 覆盖旧令牌：不然连点五次「重新发送」，就有五个链接同时有效。
	token, err := a.issueToken(ctx, user.ID, model.TokenEmailVerify, EmailVerifyTTL, true)
	if err != nil {
		return err
	}
	a.sendVerification(ctx, user.Email, token)
	return nil
}

type LoginResult struct {
	User      model.User
	Token     string
	ExpiresAt time.Time
}

func (a *Account) Login(ctx context.Context, email, password, userAgent, ip string) (*LoginResult, error) {
	normalized, err := auth.NormalizeEmail(email)
	if err != nil {
		// 格式不对也要付一次哈希的代价。否则「输入不是邮箱」比「口令错误」
		// 快出 250ms，这条时间差本身就是个探测口。
		auth.VerifyPassword(auth.DummyPasswordHash, password)
		return nil, ErrCredentialsInvalid
	}

	user, err := a.store.GetUserByEmail(ctx, normalized)
	if errors.Is(err, repository.ErrNotFound) {
		auth.VerifyPassword(auth.DummyPasswordHash, password)
		return nil, ErrCredentialsInvalid
	}
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(user.PasswordHash, password) {
		return nil, ErrCredentialsInvalid
	}

	// 状态和验证都放在口令之后判断。这两个错误只会发给口令已经输对的人，
	// 所以它们说的不是「这个邮箱存在」，而是「你知道这个账号的口令」——
	// 而知道口令的人本来就有资格知道这些。
	if user.Status != model.UserStatusActive {
		return nil, ErrAccountDisabled
	}
	if user.EmailVerifiedAt == nil {
		return nil, ErrEmailNotVerified
	}

	token, expiresAt, err := a.sessions.Start(ctx, user.ID, userAgent, ip)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: *user, Token: token, ExpiresAt: expiresAt}, nil
}

func (a *Account) Logout(ctx context.Context, token string) error {
	return a.sessions.End(ctx, token)
}

func (a *Account) RequestPasswordReset(ctx context.Context, email string) error {
	normalized, err := auth.NormalizeEmail(email)
	if err != nil {
		return err
	}

	user, err := a.store.GetUserByEmail(ctx, normalized)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// 停用的账号不重置：就算改了口令也进不来，白跑一趟还多发一封信。
	if user.Status != model.UserStatusActive {
		return nil
	}

	token, err := a.issueToken(ctx, user.ID, model.TokenPasswordReset, PasswordResetTTL, true)
	if err != nil {
		return err
	}
	link := a.link("/reset-password", token)
	a.send(ctx, user.Email, "重置口令",
		"点开下面的链接设置新口令（"+humanTTL(PasswordResetTTL)+"内有效）：\n\n"+link+
			"\n\n如果这不是你做的，忽略这封邮件即可，你的口令不会有任何变化。")
	return nil
}

func (a *Account) ResetPassword(ctx context.Context, token, password string) error {
	// 先做不需要令牌就能做的检查。口令太短是最常见的输入错误，
	// 不能让一次手滑就烧掉一个刚刚收到的链接。
	if err := auth.ValidatePassword(password, ""); err != nil {
		return err
	}

	consumed, err := a.consumeToken(ctx, model.TokenPasswordReset, token)
	if err != nil {
		return err
	}

	user, err := a.store.GetUserByID(ctx, consumed.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrTokenInvalid
	}
	if err != nil {
		return err
	}
	// 「口令不能和邮箱相同」要拿到用户才判断得了，走到这里令牌已经烧了。
	// 这个代价可以接受：改一下再申请一封，前后不过几秒。
	if err := auth.ValidatePassword(password, user.Email); err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := a.store.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		return err
	}
	// 改口令的意义之一就是把别人踢下线，所以这里一个会话都不保留。
	_, err = a.store.DeleteSessionsByUser(ctx, user.ID, "")
	return err
}

// 返回被踢下线的会话数。保留 keepSessionID 是因为用户此刻正用着它——
// 改完口令发现自己也被登出了，是最容易让人以为「改坏了」的一种体验。
func (a *Account) ChangePassword(ctx context.Context, userID, keepSessionID, currentPassword, nextPassword string) (int64, error) {
	user, err := a.store.GetUserByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, ErrCredentialsInvalid
	}
	if err != nil {
		return 0, err
	}
	if !auth.VerifyPassword(user.PasswordHash, currentPassword) {
		return 0, ErrCurrentPasswordWrong
	}
	if err := auth.ValidatePassword(nextPassword, user.Email); err != nil {
		return 0, err
	}
	// 改成同一个口令，除了把其它设备踢下线没有任何作用，多半是填错了。
	if auth.VerifyPassword(user.PasswordHash, nextPassword) {
		return 0, ErrPasswordUnchanged
	}

	hash, err := auth.HashPassword(nextPassword)
	if err != nil {
		return 0, err
	}
	if err := a.store.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		return 0, err
	}
	return a.store.DeleteSessionsByUser(ctx, user.ID, keepSessionID)
}

func (a *Account) ListSessions(ctx context.Context, userID string) ([]model.Session, error) {
	return a.store.ListSessionsByUser(ctx, userID)
}

// 删除按 (userID, sessionID) 定位，不按 sessionID 单独定位：拿到别人的会话 ID
// 也删不掉别人的会话。返回 0 行表示「不存在或不属于你」，这两件事不区分。
func (a *Account) RevokeSession(ctx context.Context, userID, sessionID string) (int64, error) {
	return a.store.DeleteSession(ctx, userID, sessionID)
}

func (a *Account) issueToken(ctx context.Context, userID, kind string, ttl time.Duration, replace bool) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	id, err := auth.NewID()
	if err != nil {
		return "", err
	}

	now := a.now()
	record := &model.AuthToken{
		ID:        id,
		UserID:    userID,
		Kind:      kind,
		TokenHash: auth.TokenHash(token),
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}

	if replace {
		err = a.store.ReplaceAuthToken(ctx, record)
	} else {
		err = a.store.CreateAuthToken(ctx, record)
	}
	if err != nil {
		return "", err
	}
	return token, nil
}

func (a *Account) consumeToken(ctx context.Context, kind, token string) (*model.AuthToken, error) {
	if token == "" {
		return nil, ErrTokenInvalid
	}
	consumed, err := a.store.ConsumeAuthToken(ctx, kind, auth.TokenHash(token), a.now())
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	return consumed, nil
}

func (a *Account) sendVerification(ctx context.Context, to, token string) {
	link := a.link("/verify-email", token)
	a.send(ctx, to, "验证你的邮箱",
		"点开下面的链接完成邮箱验证（"+humanTTL(EmailVerifyTTL)+"内有效）：\n\n"+link+
			"\n\n如果这不是你做的，忽略这封邮件即可。")
}

// 发信失败不上报给调用方。这几个接口的响应体是刻意做成不区分邮箱是否存在的，
// 一旦把「发信失败」变成 500，就等于告诉对方「这个邮箱确实存在，只是我们
// 的邮件出问题了」——失败记进日志，由人来处理。
//
// 日志里带收件人是有意的：邮件没送达时，不写清楚发给谁就没法查。
func (a *Account) send(ctx context.Context, to, subject, text string) {
	if err := a.mailer.Send(ctx, mail.Message{To: to, Subject: subject, Text: text}); err != nil {
		log.Printf("发送邮件失败：收件人=%s 主题=%s 原因=%v", to, subject, err)
	}
}

func (a *Account) link(path, token string) string {
	return a.baseURL + path + "?" + url.Values{"token": {token}}.Encode()
}

// 有效期写死在正文里迟早会和常量对不上，所以从常量算出来。
func humanTTL(ttl time.Duration) string {
	switch {
	case ttl >= 24*time.Hour && ttl%(24*time.Hour) == 0:
		return fmt.Sprintf("%d 天", int(ttl/(24*time.Hour)))
	case ttl%time.Hour == 0:
		return fmt.Sprintf("%d 小时", int(ttl/time.Hour))
	default:
		return fmt.Sprintf("%d 分钟", int(ttl/time.Minute))
	}
}
