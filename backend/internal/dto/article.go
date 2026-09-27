package dto

import (
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

type ArticleSummary struct {
	ID             string    `json:"id"`
	Slug           string    `json:"slug"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	Category       string    `json:"category"`
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
	if m.Tags == nil {
		m.Tags = []string{}
	}
	return ArticleSummary{
		ID:             m.ID,
		Slug:           m.Slug,
		Title:          m.Title,
		Summary:        m.Summary,
		Category:       m.Category,
		Tags:           m.Tags,
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
