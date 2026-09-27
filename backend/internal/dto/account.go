package dto

import (
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

// 字段是挑过的：password_hash、status 不出去，email_verified_at 只以布尔形式出去
// ——前端要的是「验证了没有」，不是那个时间点。
type AccountView struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
}

type SessionView struct {
	Account   AccountView `json:"account"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

func NewAccountView(m model.User) AccountView {
	return AccountView{
		ID:            m.ID,
		Email:         m.Email,
		Role:          m.Role,
		EmailVerified: m.EmailVerifiedAt != nil,
		CreatedAt:     m.CreatedAt.UTC(),
	}
}

func NewSessionView(m model.User, expiresAt time.Time) SessionView {
	return SessionView{
		Account:   NewAccountView(m),
		ExpiresAt: expiresAt.UTC(),
	}
}

type MessageView struct {
	Message string `json:"message"`
}

func NewMessage(message string) MessageView {
	return MessageView{Message: message}
}

// 登录设备列表。IP 和 User-Agent 都出去——它们本来就是这个列表存在的理由：
// 让人认出不认识的那一台。token_hash 不出去，那不是给前端看的东西。
type DeviceView struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"userAgent"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

func NewDeviceView(m model.Session, current bool) DeviceView {
	return DeviceView{
		ID:        m.ID,
		UserAgent: m.UserAgent,
		IP:        m.IP,
		CreatedAt: m.CreatedAt.UTC(),
		ExpiresAt: m.ExpiresAt.UTC(),
		Current:   current,
	}
}

type DeviceListView struct {
	Sessions []DeviceView `json:"sessions"`
}
