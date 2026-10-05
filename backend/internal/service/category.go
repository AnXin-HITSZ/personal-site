package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

var (
	ErrCategoryNotFound      = errors.New("分类不存在")
	ErrCategoryNameTaken     = errors.New("已经有一个同名的分类了")
	ErrCategoryNameRequired  = errors.New("分类名不能为空")
	ErrCategoryNameTooLong   = fmt.Errorf("分类名不能超过 %d 个字符", categoryNameMaxRunes)
	ErrTooManyCategories     = fmt.Errorf("分类最多 %d 个", maxCategories)
	ErrCategoryInUse         = errors.New("这个分类下面还有文章，删掉之前得先给它们找个去处")
	ErrCategoryMoveToSelf    = errors.New("不能把文章挪到它自己下面")
	ErrCategoryOrderMismatch = errors.New("提交的顺序和现有的分类对不上，请重新加载")
)

const (
	categoryNameMaxRunes = 16

	// 读者那一行摆不下更多了，这一页也就不用再多。
	maxCategories = 20

	maxCategoryIDAttempts = 3
)

// 分类 id 出现在两个地方：数据库里，和筛选地址的 ?category= 那一段。形状只用管到
// 「像个 id」——某一段是不是真的存在，由查库的结果说话（筛选时是空列表，写入时是
// 外键）。内置那四个短代号清掉之后，id 一律是 Create 现取的 8 位随机码（和文章 id
// 同款），所以长度就是 8，不必再放宽。
var categoryIDPattern = regexp.MustCompile(`^[0-9a-z]{8}$`)

func IsCategoryID(id string) bool {
	return categoryIDPattern.MatchString(id)
}

// 后台要能注入假实现来测，所以服务只认这个接口，不认具体的仓储。
type CategoryStore interface {
	ListWithUsage(ctx context.Context) ([]repository.CategoryRow, error)
	Create(ctx context.Context, category *model.Category, limit int) error
	Rename(ctx context.Context, id, name string, updatedAt time.Time) error
	Reorder(ctx context.Context, ids []string, updatedAt time.Time) error
	Delete(ctx context.Context, id string) error
	DeleteAndMove(ctx context.Context, id, moveTo string, updatedAt time.Time) error
}

type Category struct {
	repo CategoryStore
	now  func() time.Time
}

func NewCategory(repo CategoryStore) *Category {
	return &Category{repo: repo, now: time.Now}
}

// 读者那一行只列有已发布文章的分类：筛出来是空的按钮只是占位，和「列表空着的时候
// 不出筛选器」同一条道理。所以一篇都没发出去过的分类在写作那一页看得见、在读者
// 这边看不见。带上「已发布几篇」「最近什么时候」——分类页直接摆这两个数。
func (s *Category) ListPublished(ctx context.Context) (dto.CategoryList, error) {
	rows, err := s.repo.ListWithUsage(ctx)
	if err != nil {
		return dto.CategoryList{}, err
	}

	items := make([]dto.CategoryRef, 0, len(rows))
	for _, row := range rows {
		if row.Published == 0 {
			continue
		}
		items = append(items, dto.CategoryRef{
			ID:                row.ID,
			Name:              row.Name,
			ArticleCount:      row.Published,
			LatestPublishedAt: row.LatestPublishedAt,
		})
	}

	return dto.CategoryList{Items: items}, nil
}

func (s *Category) ListAdmin(ctx context.Context) (dto.AdminCategoryList, error) {
	rows, err := s.repo.ListWithUsage(ctx)
	if err != nil {
		return dto.AdminCategoryList{}, err
	}

	items := make([]dto.AdminCategory, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminCategory(row))
	}

	return dto.AdminCategoryList{Items: items}, nil
}

func (s *Category) Create(ctx context.Context, name string) (dto.AdminCategory, error) {
	normalized, err := normalizeCategoryName(name)
	if err != nil {
		return dto.AdminCategory{}, err
	}

	now := s.now().UTC()
	// 和文章 id 同一个来路：40 位随机撞一次是极小概率，撞两次基本说明随机源出了问题
	// ——与其无限重试，不如报出来。
	for attempt := 0; attempt < maxCategoryIDAttempts; attempt++ {
		id, err := auth.NewShortID()
		if err != nil {
			return dto.AdminCategory{}, err
		}

		category := &model.Category{ID: id, Name: normalized, CreatedAt: now, UpdatedAt: now}
		err = s.repo.Create(ctx, category, maxCategories)
		if err == nil {
			// 刚建出来的分类下面不可能有文章，两个计数不用再查一次。
			return dto.AdminCategory{ID: category.ID, Name: category.Name}, nil
		}
		if !errors.Is(err, repository.ErrIDTaken) {
			return dto.AdminCategory{}, categoryWriteError(err)
		}
	}

	return dto.AdminCategory{}, fmt.Errorf("连续 %d 次撞上同一个主键", maxCategoryIDAttempts)
}

func (s *Category) Rename(ctx context.Context, id, name string) (dto.AdminCategory, error) {
	normalized, err := normalizeCategoryName(name)
	if err != nil {
		return dto.AdminCategory{}, err
	}

	// 先读一次，既是为了「分类不存在」能报成 404，也是为了拿到两个计数一起回给前端
	// ——改名不动任何一篇文章，所以刚读到的数就是改完之后的数。
	current, err := s.find(ctx, id)
	if err != nil {
		return dto.AdminCategory{}, err
	}

	if err := categoryWriteError(s.repo.Rename(ctx, id, normalized, s.now().UTC())); err != nil {
		return dto.AdminCategory{}, err
	}

	current.Name = normalized
	return adminCategory(current), nil
}

func (s *Category) Reorder(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if !IsCategoryID(id) {
			return ErrCategoryOrderMismatch
		}
	}
	return categoryWriteError(s.repo.Reorder(ctx, ids, s.now().UTC()))
}

// moveTo 为空表示「这个分类下面本来就没有文章」。有文章又不给去处，就不动手——
// 这不是前端拦的，外键也拦着，这里只是把话说在前面。
func (s *Category) Delete(ctx context.Context, id, moveTo string) error {
	current, err := s.find(ctx, id)
	if err != nil {
		return err
	}

	if moveTo == "" {
		if current.Total > 0 {
			return ErrCategoryInUse
		}
		return categoryWriteError(s.repo.Delete(ctx, id))
	}

	if moveTo == id {
		return ErrCategoryMoveToSelf
	}
	if !IsCategoryID(moveTo) {
		return ErrCategoryNotFound
	}
	return categoryWriteError(s.repo.DeleteAndMove(ctx, id, moveTo, s.now().UTC()))
}

func (s *Category) find(ctx context.Context, id string) (repository.CategoryRow, error) {
	rows, err := s.repo.ListWithUsage(ctx)
	if err != nil {
		return repository.CategoryRow{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, nil
		}
	}
	return repository.CategoryRow{}, ErrCategoryNotFound
}

func adminCategory(row repository.CategoryRow) dto.AdminCategory {
	return dto.AdminCategory{
		ID:           row.ID,
		Name:         row.Name,
		ArticleCount: row.Total,
		DraftCount:   row.Drafts,
	}
}

// 名字不进任何地址，所以除了长度没有别的字符要求——去掉首尾空格之后不为空就行。
// 撞名不在这里查：那是唯一索引的事，而且它由库判、大小写不敏感（utf8mb4_0900_as_ci），
// 代码里再比一遍只会比出第二套规则。
func normalizeCategoryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		return "", ErrCategoryNameRequired
	case utf8.RuneCountInString(trimmed) > categoryNameMaxRunes:
		return "", ErrCategoryNameTooLong
	}
	return trimmed, nil
}

// 把仓储的错误翻成这一层的说法，handler 就不必认识 repository 包。
//
// 两个「不存在」都要接住：ErrNotFound 是分类本身没查到，ErrCategoryNotFound 是
// 挪文章时那个去处不在了——挪过去之前只验过它像不像 id，它是不是还在得由外键说。
func categoryWriteError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrCategoryNotFound):
		return ErrCategoryNotFound
	case errors.Is(err, repository.ErrCategoryNameTaken):
		return ErrCategoryNameTaken
	case errors.Is(err, repository.ErrCategoryInUse):
		return ErrCategoryInUse
	case errors.Is(err, repository.ErrTooManyCategories):
		return ErrTooManyCategories
	case errors.Is(err, repository.ErrOrderMismatch):
		return ErrCategoryOrderMismatch
	default:
		return err
	}
}
