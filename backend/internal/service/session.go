package service

import (
	"context"
	"errors"
	"time"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

const (
	// 到期不续：绝对过期，每次请求不写库。
	SessionTTL = 30 * 24 * time.Hour

	// 没用 __Host- 前缀：那个前缀要求同时满足 Secure、Path=/、不带 Domain，
	// 而开发环境走 http，Secure 必须为 false，两边就共用一个名字了。
	SessionCookieName = "axh_sid"
)

var ErrSessionInvalid = errors.New("会话无效")

// 仓储层是具体类型，这里收窄成接口：中间件要能用假实现做 httptest，
// 不能为了跑测试去连一个数据库。
type SessionStore interface {
	CreateSession(ctx context.Context, session *model.Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
}

type Authenticated struct {
	User    model.User
	Session model.Session
}

type Session struct {
	store SessionStore
	now   func() time.Time
}

func NewSession(store SessionStore) *Session {
	return &Session{store: store, now: time.Now}
}

// 返回明文令牌——它只在这一刻存在：写进 cookie，交给浏览器；库里存的是它的哈希。
// 到期时刻一并返回，是因为登录响应里要告诉前端会话什么时候失效；让调用方自己
// 用 time.Now() 再算一遍，两边就会差那么几微秒——数字对不上的时候没人愿意去查原因。
func (s *Session) Start(ctx context.Context, userID, userAgent, ip string) (string, time.Time, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", time.Time{}, err
	}
	id, err := auth.NewID()
	if err != nil {
		return "", time.Time{}, err
	}

	now := s.now()
	session := &model.Session{
		ID:        id,
		UserID:    userID,
		TokenHash: auth.TokenHash(token),
		UserAgent: userAgent,
		IP:        ip,
		ExpiresAt: now.Add(SessionTTL),
		CreatedAt: now,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return "", time.Time{}, err
	}
	return token, session.ExpiresAt, nil
}

// 令牌不存在、会话已过期、用户已被删、账号被停用，都归成 ErrSessionInvalid。
// 但仓储报出来的其它错误要原样往上传——数据库连不上不是「会话无效」，
// 把它吞成 401 会让一次故障看起来像所有人都被登出了。
func (s *Session) Authenticate(ctx context.Context, token string) (*Authenticated, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	session, err := s.store.GetSessionByTokenHash(ctx, auth.TokenHash(token))
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, err
	}

	if !session.ExpiresAt.After(s.now()) {
		return nil, ErrSessionInvalid
	}

	user, err := s.store.GetUserByID(ctx, session.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, ErrSessionInvalid
	}

	return &Authenticated{User: *user, Session: *session}, nil
}

// 退出登录只关心结果：令牌本来就不存在（重复点退出、cookie 已过期）不算错误。
func (s *Session) End(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSessionByTokenHash(ctx, auth.TokenHash(token))
}
