package model

import "time"

// 只有这两态，draft 不出现在任何公开接口里。
const (
	ArticleStatusDraft     = "draft"
	ArticleStatusPublished = "published"
)

// Article 是一篇文章，读者和写作台读的是同一行：草稿与已发布同在一张表，由 status 分开。
//
// 几处不是一眼看得出来的：slug 有唯一索引，但它只是地址里给人看的那一段，查库一律按 id，
// 所以改它不作废任何既有链接；category 存的是分类的 id，外键指向 categories 且是 RESTRICT，
// 还有文章的分类删不掉；published_at 可空，这个「空」就是「还没发布」，转回草稿时它保留
// 原值，重新发布便还能留住首发时间；reading_minutes 与 body_runes 都由服务端按正文算好
// 落库，而不是每次现算——列表不读正文，现算不出来。
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
