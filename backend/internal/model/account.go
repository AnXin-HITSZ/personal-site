package model

import "time"

// 注册只产生 member；admin 由部署时的一次性命令指定，没有任何接口能改动这个字段。
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// status 没有接口能改，是留给运维的开关：把账号锁住只需一条 UPDATE。
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

const (
	TokenEmailVerify   = "email_verify"
	TokenPasswordReset = "password_reset"
)

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

type Session struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"not null"`
	TokenHash string `gorm:"not null;unique"`
	UserAgent string `gorm:"not null"`
	IP        string `gorm:"not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}

type AuthToken struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"not null"`
	Kind      string `gorm:"not null"`
	TokenHash string `gorm:"not null;unique"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
