package model

import "time"

// 只有这两态，draft 不出现在任何公开接口里。
const (
	ArticleStatusDraft     = "draft"
	ArticleStatusPublished = "published"
)

type Article struct {
	ID             string `gorm:"primaryKey"`
	Slug           string `gorm:"not null;unique"`
	Title          string `gorm:"not null"`
	Summary        string `gorm:"not null"`
	Body           string `gorm:"not null;type:mediumtext"`
	Category       string
	Tags           []string `gorm:"serializer:json;type:json"`
	Status         string
	PublishedAt    *time.Time
	ReadingMinutes int
	BodyRunes      int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
