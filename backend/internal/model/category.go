package model

import "time"

// 文章在列表和详情里存的是分类的 id，不是名字：改名字不动任何一篇文章，
// 也不动任何一条链接。这也是这一站唯一一张可以被写接口改动的表。
type Category struct {
	ID        string `gorm:"primaryKey"`
	Name      string `gorm:"not null;unique"`
	SortOrder int    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
