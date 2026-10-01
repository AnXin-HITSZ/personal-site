package model

import "time"

// 注册只产生 member；admin 由部署时的一次性命令指定，没有任何接口能改动这个字段。
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// status 没有接口能改，是留给运维的开关：把账号锁住只需一条 UPDATE。它不只拦住下一次
// 登录——已经发出去的会话也会在下一次请求上失效，因为认证每次都回查一遍用户。
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// 两类邮件令牌：注册后验证邮箱、忘记口令时重设。两者都是一次性的，用掉的那一刻记在
// AuthToken.UsedAt 上。
const (
	TokenEmailVerify   = "email_verify"
	TokenPasswordReset = "password_reset"
)

// User 是一个人。email_verified_at 可空，而这个「空」本身就是状态：没验证过邮箱的账号
// 不能登录，不必另开一列去记。
type User struct {
	ID              string `gorm:"primaryKey"`
	Email           string `gorm:"not null;unique"`
	PasswordHash    string `gorm:"not null"`
	Role            string `gorm:"not null"`
	Status          string `gorm:"not null"`
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Session 是一次登录。整行由 token_hash 的唯一索引进出：cookie 里装的是原令牌，取会话时
// 现算一次哈希再查，库里没有一个能直接拿去用的凭据。
//
// user_agent 与 ip 只为「我的账号」里那串登录记录而存，鉴权不读它们；UA 过长会在入库前
// 按字符数截断（见 repository）。
type Session struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"not null"`
	TokenHash string `gorm:"not null;unique"`
	UserAgent string `gorm:"not null"`
	IP        string `gorm:"not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}

// AuthToken 是发到邮箱里的一次性令牌。used_at 记下它被用掉的那一刻，之后不再认它：
// 验证邮件被转发、链接留在浏览器历史里，都不至于变成第二次机会。
type AuthToken struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"not null"`
	Kind      string `gorm:"not null"`
	TokenHash string `gorm:"not null;unique"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
