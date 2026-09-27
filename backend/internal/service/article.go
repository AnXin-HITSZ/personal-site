package service

import (
	"context"
	"errors"
	"fmt"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/repository"
)

var ErrArticleNotFound = errors.New("文章不存在")

type Article struct {
	repo *repository.Article
}

func NewArticle(repo *repository.Article) *Article {
	return &Article{repo: repo}
}

func (s *Article) ListPublished(ctx context.Context, query dto.ArticleListQuery) (dto.ArticleList, error) {
	if query.Page < 1 || query.PageSize < 1 {
		return dto.ArticleList{}, fmt.Errorf("分页参数非法: page=%d pageSize=%d", query.Page, query.PageSize)
	}

	articles, total, err := s.repo.ListPublished(
		ctx, query.Keyword, query.Category, query.PageSize, (query.Page-1)*query.PageSize)
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
			Page:       query.Page,
			PageSize:   query.PageSize,
			Total:      total,
			TotalPages: (total + query.PageSize - 1) / query.PageSize,
		},
	}, nil
}

func (s *Article) GetPublished(ctx context.Context, slug string) (dto.ArticleDetail, error) {
	article, err := s.repo.GetPublishedBySlug(ctx, slug)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.ArticleDetail{}, ErrArticleNotFound
	}
	if err != nil {
		return dto.ArticleDetail{}, err
	}

	detail, ok := dto.NewArticleDetail(*article)
	if !ok {
		return dto.ArticleDetail{}, fmt.Errorf("文章 %s 状态为 published 但 published_at 为空", article.ID)
	}

	return detail, nil
}
