package dto

import "time"

// 读者那一行说三件事：id 是筛选用 ?category= 时带的那一段，名字是按钮上写的字，
// 还有分类页要摆的「N 篇」「最近某天」。篇数只数已发布的——读者看不见草稿。
type CategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// 这个分类下已经发布的篇数。有已发布的文章才会有这一行，所以至少是 1。
	ArticleCount int `json:"articleCount"`
	// 最近一篇的发布时间，即分组里 published_at 的最大值。
	LatestPublishedAt *time.Time `json:"latestPublishedAt"`
}

type CategoryList struct {
	Items []CategoryRef `json:"items"`
}

// 写作那边多两个数：这一行要显示「4 篇」和「其中 1 篇草稿」，删除框里也要拿它们
// 说清楚会牵动几篇。读者不需要知道这些，所以两个 DTO 不合并。
type AdminCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// 归在这个分类下的全部文章，草稿也算。
	ArticleCount int `json:"articleCount"`
	DraftCount   int `json:"draftCount"`
}

type AdminCategoryList struct {
	Items []AdminCategory `json:"items"`
}
