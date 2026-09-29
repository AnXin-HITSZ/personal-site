package dto

// 读者那一行只认 id 和名字。id 是筛选用 ?category= 时带的那一段，名字是按钮上写的字。
type CategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
