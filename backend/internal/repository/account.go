package repository

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/model"
)

var ErrEmailTaken = errors.New("邮箱已被注册")

const errnoDuplicateEntry = 1062

// 只看错误号，不看错误正文：正文里有触发冲突的那行数据（注册时就是邮箱），
// 而这是要写进日志和响应体的东西。
func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == errnoDuplicateEntry
}

// user_agent 与 ip 是诊断信息，不是身份。超长时截断比报错合适——
// 列宽是字符数，所以按字符截，不是按字节。
func truncateRunes(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	count := 0
	for i := range s {
		if count == max {
			return s[:i]
		}
		count++
	}
	return s
}

const (
	maxUserAgentRunes = 255
	maxIPRunes        = 45
)

type Account struct {
	db *gorm.DB
}

func NewAccount(db *gorm.DB) *Account {
	return &Account{db: db}
}

func (r *Account) CreateUser(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	// users 上只有 email 一个唯一索引，所以这里的冲突只可能是邮箱重复。
	if isDuplicateEntry(err) {
		return ErrEmailTaken
	}
	return err
}

func (r *Account) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Account) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// 带 email_verified_at IS NULL 的条件，重复点验证链接不会把首次验证时间改掉。
func (r *Account) MarkEmailVerified(ctx context.Context, userID string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ? AND email_verified_at IS NULL", userID).
		Update("email_verified_at", at).Error
}

func (r *Account) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Update("password_hash", hash).Error
}

// 唯一能把 role 写成 admin 的地方，只被 -create-admin 调用——接口层没有任何路径
// 能到达这里，所以「注册只产生 member」是结构上成立的，不是靠自觉。
//
// 一条 UPDATE 写四个字段，是为了不给「改了 role 但口令还是旧的」留出中间状态。
// status 一并放开：被封的账号提成管理员却还是登不进来，只会让人以为命令没生效。
func (r *Account) MakeAdmin(ctx context.Context, userID, passwordHash string, verifiedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"password_hash":     passwordHash,
			"role":              model.RoleAdmin,
			"status":            model.UserStatusActive,
			"email_verified_at": verifiedAt,
		}).Error
}

func (r *Account) CreateSession(ctx context.Context, session *model.Session) error {
	session.UserAgent = truncateRunes(session.UserAgent, maxUserAgentRunes)
	session.IP = truncateRunes(session.IP, maxIPRunes)
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *Account) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error) {
	var session model.Session
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// 条件里带 user_id：退出登录的 id 来自请求体，不带这一条就能退掉别人的会话。
func (r *Account) DeleteSession(ctx context.Context, userID, sessionID string) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, sessionID).
		Delete(&model.Session{})
	return res.RowsAffected, res.Error
}

func (r *Account) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	return r.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		Delete(&model.Session{}).Error
}

// exceptID 传当前会话的 id：改密后保留自己，踢掉其它设备。
func (r *Account) DeleteSessionsByUser(ctx context.Context, userID, exceptID string) (int64, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if exceptID != "" {
		query = query.Where("id <> ?", exceptID)
	}
	res := query.Delete(&model.Session{})
	return res.RowsAffected, res.Error
}

func (r *Account) ListSessionsByUser(ctx context.Context, userID string) ([]model.Session, error) {
	var sessions []model.Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *Account) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("expires_at <= ?", now).
		Delete(&model.Session{})
	return res.RowsAffected, res.Error
}

func (r *Account) CreateAuthToken(ctx context.Context, token *model.AuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// 同一种令牌同时只有一个有效：发新的就作废旧的，用户连点两次「重发」不会留下两把钥匙。
// 删除和插入必须在一个事务里，否则中间失败会两把都没了。
func (r *Account) ReplaceAuthToken(ctx context.Context, token *model.AuthToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND kind = ?", token.UserID, token.Kind).
			Delete(&model.AuthToken{}).Error
		if err != nil {
			return err
		}
		return tx.Create(token).Error
	})
}

// 一次性消费：先用一条带全部条件的 UPDATE 抢占，影响行数为 1 才算抢到。
// 改成先 SELECT 再 UPDATE 会留下竞态——两个请求可以同时读到「还没用过」。
// 这里 used_at 从 NULL 变成时间，行一定发生变化，所以 RowsAffected 不依赖
// 驱动的 ClientFoundRows 设置。
//
// 令牌不存在、已用过、已过期、kind 不符，返回的都是 ErrNotFound：调用方
// 只需要知道这个链接无效。
func (r *Account) ConsumeAuthToken(ctx context.Context, kind, tokenHash string, now time.Time) (*model.AuthToken, error) {
	var token model.AuthToken
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.AuthToken{}).
			Where("token_hash = ? AND kind = ? AND used_at IS NULL AND expires_at > ?", tokenHash, kind, now).
			Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		// 抢占成功的那一行被本事务锁住，读回来一定是最新的。
		return tx.Where("token_hash = ?", tokenHash).Take(&token).Error
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}
