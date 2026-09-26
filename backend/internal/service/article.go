package service

import (
	"context"
	"fmt"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/repository"
)

type Article struct {
	repo *repository.Article
}

func NewArticle(repo *repository.Article) *Article {
	return &Article{repo: repo}
}

func (s *Article) ListPublished(ctx context.Context, page, pageSize int) (dto.ArticleList, error) {
	if page < 1 || pageSize < 1 {
		return dto.ArticleList{}, fmt.Errorf("分页参数非法: page=%d pageSize=%d", page, pageSize)
	}

	articles, total, err := s.repo.ListPublished(ctx, pageSize, (page-1)*pageSize)
	if err != nil {
		return dto.ArticleList{}, err
	}

	items := make([]dto.ArticleSummary, 0, len(articles))
	for _, article := range articles {
		summary, ok := dto.NewArticleSummary(article)
		if !ok {
			return dto.ArticleList{}, fmt.Errorf("文章 %s 状态为 published 但 published_at 为空", article.ID)
		}
		items = append(items, summary)
	}

	return dto.ArticleList{
		Items: items,
		Pagination: dto.Pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: (total + pageSize - 1) / pageSize,
		},
	}, nil
}
