package dto

import (
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

type ArticleSummary struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// category 是筛选用 ?category= 时带的那一段，categoryName 是页面上写出来的
	// 那几个字。两个都要：一个是地址里的，一个是给人看的。
	Category       string    `json:"category"`
	CategoryName   string    `json:"categoryName"`
	Tags           []string  `json:"tags"`
	PublishedAt    time.Time `json:"publishedAt"`
	ReadingMinutes int       `json:"readingMinutes"`
}

type ArticleDetail struct {
	ArticleSummary
	Body string `json:"body"`
}

type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type ArticleList struct {
	Items      []ArticleSummary `json:"items"`
	Pagination Pagination       `json:"pagination"`
}

type ArticleListQuery struct {
	Page     int
	PageSize int
	Keyword  string
	Category string
}

func NewArticleSummary(m model.Article) (ArticleSummary, bool) {
	if m.PublishedAt == nil {
		return ArticleSummary{}, false
	}
	return ArticleSummary{
		ID:             m.ID,
		Slug:           m.Slug,
		Title:          m.Title,
		Summary:        m.Summary,
		Category:       m.Category,
		CategoryName:   m.CategoryName,
		Tags:           normalizeTags(m.Tags),
		PublishedAt:    m.PublishedAt.UTC(),
		ReadingMinutes: m.ReadingMinutes,
	}, true
}

func NewArticleDetail(m model.Article) (ArticleDetail, bool) {
	summary, ok := NewArticleSummary(m)
	if !ok {
		return ArticleDetail{}, false
	}
	return ArticleDetail{ArticleSummary: summary, Body: m.Body}, true
}

// 草稿没有发布时刻，而 ArticleSummary 的 PublishedAt 不可空——那是给读者看的，
// 能出现在那里的必然已经发布，拿不到值就是真的坏了。后台这一族不能沿用那个假设，
// 所以 PublishedAt 用指针，构造它也不返回 ok：草稿的 nil 是正常数据，不是违规。
// BodyRunes 同样只在这一族出现：读者看的是「几分钟」，字数只有写的人关心。
type AdminArticleSummary struct {
	ID             string     `json:"id"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Category       string     `json:"category"`
	Tags           []string   `json:"tags"`
	Status         string     `json:"status"`
	PublishedAt    *time.Time `json:"publishedAt"`
	ReadingMinutes int        `json:"readingMinutes"`
	BodyRunes      int        `json:"bodyRunes"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type AdminArticleDetail struct {
	AdminArticleSummary
	Body string `json:"body"`
}

type AdminArticleList struct {
	Items      []AdminArticleSummary `json:"items"`
	Pagination Pagination            `json:"pagination"`
}

type AdminArticleListQuery struct {
	Page     int
	PageSize int
	Status   string
	Category string
	Keyword  string
}

func NewAdminArticleSummary(m model.Article) AdminArticleSummary {
	var published *time.Time
	if m.PublishedAt != nil {
		utc := m.PublishedAt.UTC()
		published = &utc
	}
	return AdminArticleSummary{
		ID:             m.ID,
		Slug:           m.Slug,
		Title:          m.Title,
		Summary:        m.Summary,
		Category:       m.Category,
		Tags:           normalizeTags(m.Tags),
		Status:         m.Status,
		PublishedAt:    published,
		ReadingMinutes: m.ReadingMinutes,
		BodyRunes:      m.BodyRunes,
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}
}

func NewAdminArticleDetail(m model.Article) AdminArticleDetail {
	return AdminArticleDetail{AdminArticleSummary: NewAdminArticleSummary(m), Body: m.Body}
}

type UploadedImage struct {
	URL         string `json:"url"`
	Key         string `json:"key"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"contentType"`
}

// 参数是一个个传进来的，而不是直接收 storage.Object——dto 不该认识存储层。
func NewUploadedImage(url, key string, size int64, contentType string) UploadedImage {
	return UploadedImage{URL: url, Key: key, Bytes: size, ContentType: contentType}
}

// nil 切片会被序列化成 null，前端拿到 null 再 .map 就炸了。
func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
