package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"anxin-hitsz.com/backend/internal/model"
)

var (
	ErrCategoryNameTaken = errors.New("分类名已被占用")
	ErrCategoryInUse     = errors.New("分类下面还有文章")
	ErrTooManyCategories = errors.New("分类数量已达上限")
	ErrOrderMismatch     = errors.New("提交的顺序和库里的分类对不上")
)

// 分类自己的字段，加上它下面有多少篇文章。两个数一起读，是因为列表要同时显示
// 「4 篇」和「其中 1 篇草稿」——分两次读就会有那么一瞬间它们对不上。
type CategoryRow struct {
	ID     string
	Name   string
	Total  int
	Drafts int
}

type Category struct {
	db *gorm.DB
}

func NewCategory(db *gorm.DB) *Category {
	return &Category{db: db}
}

func (r *Category) ListWithUsage(ctx context.Context) ([]CategoryRow, error) {
	rows := make([]CategoryRow, 0, 8)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var categories []model.Category
		if err := tx.Order("sort_order asc").Order("id asc").Find(&categories).Error; err != nil {
			return err
		}

		var totals, drafts []categoryCount
		if err := tx.Model(&model.Article{}).
			Select("category, COUNT(*) AS n").Group("category").Find(&totals).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Article{}).
			Where("status = ?", model.ArticleStatusDraft).
			Select("category, COUNT(*) AS n").Group("category").Find(&drafts).Error; err != nil {
			return err
		}

		byID := make(map[string]CategoryRow, len(categories))
		for _, category := range categories {
			byID[category.ID] = CategoryRow{ID: category.ID, Name: category.Name}
		}
		for _, count := range totals {
			row := byID[count.CategoryID]
			row.Total = count.N
			byID[count.CategoryID] = row
		}
		for _, count := range drafts {
			row := byID[count.CategoryID]
			row.Drafts = count.N
			byID[count.CategoryID] = row
		}
		for _, category := range categories {
			rows = append(rows, byID[category.ID])
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

	return rows, nil
}

// categoryCount 是上面那两条分组统计的落点。列名用标签写死，不靠字段名去对。
type categoryCount struct {
	CategoryID string `gorm:"column:category"`
	N          int    `gorm:"column:n"`
}

// 上限在事务里判，不在服务层判：两次并发的「加上」都先数到 19、再各插一行，
// 就是 21 个。COUNT 上加 FOR UPDATE 会把已有的分类行锁住，让这两次排成一队。
// 为二十来行的一张表付这点代价，换的是一个不会自己长过头的上限。
func (r *Category) Create(ctx context.Context, category *model.Category, limit int) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Category{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrTooManyCategories
		}

		// 新分类排在最后。取最后一名的位置而不是 MAX(...) + 1：空表上 MAX 是 NULL，
		// 拿回来还要再判一次空；取不到行本来就意味着这是第一个，位置就是 0。
		// 上一次的排位在同一个事务里读，两个并发的新建不会拿到同一个位置
		// （上面那句 FOR UPDATE 已经把队列排好了）。
		var last []int
		if err := tx.Model(&model.Category{}).Order("sort_order desc").Limit(1).
			Pluck("sort_order", &last).Error; err != nil {
			return err
		}
		if len(last) > 0 {
			category.SortOrder = last[0] + 1
		}

		return tx.Create(category).Error
	})
	return categoryKeyError(err)
}

// 改的只有那一格。RowsAffected 和文章那边一样不能当「记录不存在」用：名字原样
// 存回去时报 0，而那不是错误。所以在服务层先查过存在性，这里不管行数。
func (r *Category) Rename(ctx context.Context, id, name string, updatedAt time.Time) error {
	return categoryKeyError(r.db.WithContext(ctx).Model(&model.Category{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "updated_at": updatedAt.UTC()}).Error)
}

// 顺序是整份提交的：一次请求把整张表写一遍，一个事务。两次单独的对调请求中间
// 断一次，顺序就停在只换了一半的地方。
func (r *Category) Reorder(ctx context.Context, ids []string, updatedAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []string
		if err := tx.Model(&model.Category{}).Order("sort_order asc").Order("id asc").
			Pluck("id", &current).Error; err != nil {
			return err
		}

		// 提交上来的必须是现在这一整份，不多不少。少一个就会有一个分类拿不到位置，
		// 多一个说明它是在这一页打开之后才建的——两种都不该猜，让前端重新读一遍。
		if len(current) != len(ids) {
			return ErrOrderMismatch
		}
		known := make(map[string]struct{}, len(current))
		for _, id := range current {
			known[id] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := known[id]; !ok {
				return ErrOrderMismatch
			}
		}

		for position, id := range ids {
			if err := tx.Model(&model.Category{}).Where("id = ?", id).
				Updates(map[string]any{"sort_order": position, "updated_at": updatedAt.UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Category) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Category{})
	if result.Error != nil {
		return categoryKeyError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// 一次请求做完两件事：先把这一批文章改成别的分类，再删掉这个分类。外键是 RESTRICT，
// 顺序反了就删不掉。
//
// 文章本身一个字都没被改过，所以 updated_at 一个都不动——跟着动的话，写作列表按
// 最近改动排，一挪就是好几篇一起跳到最前面，看着像今天重写了它们。UpdateColumn
// 正好跳过 GORM 那套自动填 UpdatedAt 的行为。
func (r *Category) DeleteAndMove(ctx context.Context, id, moveTo string, updatedAt time.Time) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Article{}).Where("category = ?", id).
			UpdateColumn("category", moveTo).Error; err != nil {
			return err
		}
		result := tx.Where("id = ?", id).Delete(&model.Category{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	return categoryKeyError(err)
}

func categoryKeyError(err error) error {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return err
	}
	switch mysqlErr.Number {
	case mysqlErrDuplicateEntry:
		switch {
		case strings.Contains(mysqlErr.Message, "uk_categories_name"):
			return ErrCategoryNameTaken
		case strings.Contains(mysqlErr.Message, "PRIMARY"):
			return ErrIDTaken
		}
	case mysqlErrRowIsReferenced:
		return ErrCategoryInUse
	case mysqlErrNoReferencedRow:
		return ErrCategoryNotFound
	}
	return err
}
