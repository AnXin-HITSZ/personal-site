package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/model"
)

var (
	ErrNotFound = errors.New("记录不存在")

	// 撞唯一索引是两回事：slug 撞了要用户改输入，主键撞了换一个随机数重试就行。
	// 分成两个错误，调用方才有得选。
	ErrSlugTaken = errors.New("slug 已被占用")
	ErrIDTaken   = errors.New("主键已被占用")

	// 这一篇要归的那个分类不存在。由外键告诉我们，而不是先查一遍再写——先查后写
	// 之间那个窗口里正好有人删掉这个分类的话，查得到、写不进去。
	ErrCategoryNotFound = errors.New("分类不存在")
)

// MySQL 的这两个外键错误号在这里各只有一个含义，因为全库只有一个外键。
const (
	mysqlErrDuplicateEntry  = 1062
	mysqlErrRowIsReferenced = 1451 // 父行还有子行：分类下面还有文章
	mysqlErrNoReferencedRow = 1452 // 子行指向不存在的父行：分类不存在
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
		query := tx.Model(&model.Article{}).Omit("Body").Where("status = ?", "published")
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

		if err := build(tx).
			Order("published_at desc").
			Order("id asc").
			Limit(limit).
			Offset(offset).
			Find(&articles).Error; err != nil {
			return err
		}

		return fillCategoryNames(tx, articles)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}

	return articles, int(total), nil
}

func (r *Article) GetPublishedByID(ctx context.Context, id string) (*model.Article, error) {
	var article model.Article
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, "published").
		Take(&article).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	/* 名字得再问一次分类表：上面那一条是 SELECT *，它只带得回文章自己的列。 */
	articles := []model.Article{article}
	if err := fillCategoryNames(r.db.WithContext(ctx), articles); err != nil {
		return nil, err
	}
	return &articles[0], nil
}

/*
读者那一份要多带一个分类名，而名字在另一张表上。分类最多二十个，整张读回来在

	Go 里对上，比在每条查询里 JOIN 一趟稳妥：文章的列一直是显式写出来的，JOIN 要
	在每一处都多写一列，还会让列名带上表前缀，多一个容易忘的地方。

	列表那一支的读在同一个事务里，所以名字和文章是同一个快照——一页里不会出现一半
	新一半旧的分类名。
*/
func fillCategoryNames(tx *gorm.DB, articles []model.Article) error {
	if len(articles) == 0 {
		return nil
	}

	var categories []model.Category
	if err := tx.Find(&categories).Error; err != nil {
		return err
	}
	names := make(map[string]string, len(categories))
	for _, category := range categories {
		names[category.ID] = category.Name
	}
	for i := range articles {
		articles[i].CategoryName = names[articles[i].Category]
	}
	return nil
}

// 三个字段留空都表示不限。
type AdminArticleFilter struct {
	Status   string
	Category string
	Keyword  string
}

func (r *Article) ListAdmin(ctx context.Context, filter AdminArticleFilter, limit, offset int) ([]model.Article, int, error) {
	var (
		articles []model.Article
		total    int64
	)

	build := func(tx *gorm.DB) *gorm.DB {
		query := tx.Model(&model.Article{}).Omit("Body")
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.Category != "" {
			query = query.Where("category = ?", filter.Category)
		}
		if filter.Keyword != "" {
			pattern := "%" + escapeLike(filter.Keyword) + "%"
			query = query.Where("(title LIKE ? OR summary LIKE ?)", pattern, pattern)
		}
		return query
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := build(tx).Count(&total).Error; err != nil {
			return err
		}

		return build(tx).
			Order("updated_at desc").
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

// 不限状态，草稿也要取得到；正文在这一条里读出来，编辑页要用。
func (r *Article) GetByID(ctx context.Context, id string) (*model.Article, error) {
	var article model.Article
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&article).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *Article) Create(ctx context.Context, article *model.Article) error {
	return r.uniqueKeyError(r.db.WithContext(ctx).Create(article).Error)
}

// 全量覆盖。用 map 而不是结构体更新，是因为结构体更新会跳过零值字段
// （没写 Select 时 GORM 只在 !isZero 时才带上），那样摘要清空、published_at 置回
// NULL 这类操作会静默地不生效。
type ArticleUpdate struct {
	Slug           string
	Title          string
	Summary        string
	Body           string
	Category       string
	Tags           []string
	Status         string
	PublishedAt    *time.Time
	ReadingMinutes int
	BodyRunes      int
	UpdatedAt      time.Time
}

// RowsAffected 在这里不能用：驱动没开 clientFoundRows，MySQL 报的是「改变」的行数，
// 而 updated_at 是秒精度——原样再存一次返回 0。把 0 当成记录不存在就会误报。
// 所以先数一次，数不到才是真的没有。
func (r *Article) Update(ctx context.Context, id string, update ArticleUpdate) error {
	// map 更新走的是裸绑定变量，GORM 不会替 tags 跑 serializer——直接放 []string
	// 进去，驱动会以「不支持的类型」报错。自己序列化成 JSON 文本，MySQL 会替我们
	// 把它转成 JSON 列。
	tags, err := json.Marshal(update.Tags)
	if err != nil {
		return err
	}

	fields := map[string]any{
		"slug":            update.Slug,
		"title":           update.Title,
		"summary":         update.Summary,
		"body":            update.Body,
		"category":        update.Category,
		"tags":            string(tags),
		"status":          update.Status,
		"published_at":    update.PublishedAt,
		"reading_minutes": update.ReadingMinutes,
		"body_runes":      update.BodyRunes,
		// 显式给，不让 GORM 用 NowFunc 填——那个值是本地时区的 time.Time，
		// 写进去是同一瞬间，但读回来做断言时会和 UTC 的字面量对不上。
		"updated_at": update.UpdatedAt.UTC(),
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists int64
		if err := tx.Model(&model.Article{}).Where("id = ?", id).Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		return r.uniqueKeyError(tx.Model(&model.Article{}).Where("id = ?", id).Updates(fields).Error)
	})
}

// 删除不一样：真的删掉的行一定算被改过，RowsAffected 为 0 就是没有这一行。
func (r *Article) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Article{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MySQL 的 1062 只说了「撞了唯一键」，撞的是哪个要看消息里的索引名。
func (r *Article) uniqueKeyError(err error) error {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return err
	}
	switch mysqlErr.Number {
	case mysqlErrDuplicateEntry:
		switch {
		case strings.Contains(mysqlErr.Message, "uk_articles_slug"):
			return ErrSlugTaken
		case strings.Contains(mysqlErr.Message, "PRIMARY"):
			return ErrIDTaken
		}
	case mysqlErrNoReferencedRow:
		return ErrCategoryNotFound
	}
	return err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
