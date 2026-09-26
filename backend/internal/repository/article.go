package repository

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/model"
)

type Article struct {
	db *gorm.DB
}

func NewArticle(db *gorm.DB) *Article {
	return &Article{db: db}
}

func (r *Article) ListPublished(ctx context.Context, limit, offset int) ([]model.Article, int, error) {
	var (
		articles []model.Article
		total    int64
	)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Article{}).
			Where("status = ?", "published").
			Count(&total).Error; err != nil {
			return err
		}

		return tx.Model(&model.Article{}).
			Where("status = ?", "published").
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
