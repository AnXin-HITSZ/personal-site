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

	// 分类名不在文章表上，它是每次从 categories 里现取的（见 repository 的
	// fillCategoryNames）。`gorm:"-"` 让 GORM 当它不存在：建表不管它，任何一条
	// 查询也不会去列它——所以没有哪一处需要记得把它排掉。
	CategoryName string `gorm:"-"`
}
