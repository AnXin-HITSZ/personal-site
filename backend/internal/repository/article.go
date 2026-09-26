package repository

import (
	"context"
	"database/sql"
	"strings"

	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/model"
)

type Article struct {
	db *gorm.DB
}

func NewArticle(db *gorm.DB) *Article {
	return &Article{db: db}
}

func (r *Article) ListPublished(ctx context.Context, keyword, category string, limit, offset int) ([]model.Article, int, error) {
	var (
		articles []model.Article
		total    int64
	)

	build := func(tx *gorm.DB) *gorm.DB {
		query := tx.Model(&model.Article{}).Where("status = ?", "published")
		if category != "" {
			query = query.Where("category = ?", category)
		}
		if keyword != "" {
			pattern := "%" + escapeLike(keyword) + "%"
			query = query.Where("(title LIKE ? OR summary LIKE ?)", pattern, pattern)
		}
		return query
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := build(tx).Count(&total).Error; err != nil {
			return err
		}

		return build(tx).
			Order("published_at desc").
			Order("id asc").
			Limit(limit).
			Offset(offset).
			Find(&articles).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}

	return articles, int(total), nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
