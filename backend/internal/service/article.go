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
	ErrArticleNotFound = errors.New("文章不存在")
	ErrSlugTaken       = errors.New("这个 slug 已经被别的文章用了")

	ErrInvalidSlug     = errors.New("slug 只能用英文小写字母、数字和连字符，且不能以连字符开头或结尾")
	ErrTitleRequired   = errors.New("标题不能为空")
	ErrTitleTooLong    = fmt.Errorf("标题不能超过 %d 个字符", articleTitleMaxRunes)
	ErrSummaryRequired = errors.New("摘要不能为空")
	ErrSummaryTooLong  = fmt.Errorf("摘要不能超过 %d 个字符", articleSummaryMaxRunes)
	ErrBodyRequired    = errors.New("正文不能为空")
	ErrBodyTooLong     = fmt.Errorf("正文不能超过 %d 个字符", articleBodyMaxRunes)
	ErrInvalidCategory = errors.New("未知的分类")
	ErrInvalidStatus   = errors.New("文章状态只能是 draft 或 published")
	ErrTooManyTags     = fmt.Errorf("标签最多 %d 个", maxArticleTags)
	ErrTagTooLong      = fmt.Errorf("单个标签不能超过 %d 个字符", articleTagMaxRunes)
)

const (
	articleSlugMaxRunes    = 120
	articleTitleMaxRunes   = 255
	articleSummaryMaxRunes = 500
	articleBodyMaxRunes    = 200000
	articleTagMaxRunes     = 32
	maxArticleTags         = 10

	// 中文按字数读得比英文词慢，350 是个偏保守的折算——估长了读者不亏，估短了才让人意外。
	readingRunesPerMinute = 350

	maxArticleIDAttempts = 3
)

// 留空都表示不限。
var articleCategories = map[string]struct{}{
	"backend":  {},
	"frontend": {},
	"ai":       {},
	"notes":    {},
}

func IsArticleCategory(category string) bool {
	_, ok := articleCategories[category]
	return ok
}

// 后台要能注入假实现来测，所以服务只认这个接口，不认具体的仓储。
type ArticleStore interface {
	ListPublished(ctx context.Context, keyword, category string, limit, offset int) ([]model.Article, int, error)
	GetPublishedByID(ctx context.Context, id string) (*model.Article, error)
	ListAdmin(ctx context.Context, filter repository.AdminArticleFilter, limit, offset int) ([]model.Article, int, error)
	GetByID(ctx context.Context, id string) (*model.Article, error)
	Create(ctx context.Context, article *model.Article) error
	Update(ctx context.Context, id string, update repository.ArticleUpdate) error
	Delete(ctx context.Context, id string) error
}

type Article struct {
	repo ArticleStore
	now  func() time.Time
}

func NewArticle(repo ArticleStore) *Article {
	return &Article{repo: repo, now: time.Now}
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

func (s *Article) GetPublished(ctx context.Context, id string) (dto.ArticleDetail, error) {
	article, err := s.repo.GetPublishedByID(ctx, id)
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

type ArticleInput struct {
	Slug     string
	Title    string
	Summary  string
	Body     string
	Category string
	Tags     []string
	Status   string
}

func (s *Article) ListAdmin(ctx context.Context, query dto.AdminArticleListQuery) (dto.AdminArticleList, error) {
	if query.Page < 1 || query.PageSize < 1 {
		return dto.AdminArticleList{}, fmt.Errorf("分页参数非法: page=%d pageSize=%d", query.Page, query.PageSize)
	}

	articles, total, err := s.repo.ListAdmin(ctx, repository.AdminArticleFilter{
		Status:   query.Status,
		Category: query.Category,
		Keyword:  query.Keyword,
	}, query.PageSize, (query.Page-1)*query.PageSize)
	if err != nil {
		return dto.AdminArticleList{}, err
	}

	items := make([]dto.AdminArticleSummary, 0, len(articles))
	for _, article := range articles {
		items = append(items, dto.NewAdminArticleSummary(article))
	}

	return dto.AdminArticleList{
		Items: items,
		Pagination: dto.Pagination{
			Page:       query.Page,
			PageSize:   query.PageSize,
			Total:      total,
			TotalPages: (total + query.PageSize - 1) / query.PageSize,
		},
	}, nil
}

func (s *Article) GetForEdit(ctx context.Context, id string) (dto.AdminArticleDetail, error) {
	article, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.AdminArticleDetail{}, ErrArticleNotFound
	}
	if err != nil {
		return dto.AdminArticleDetail{}, err
	}
	return dto.NewAdminArticleDetail(*article), nil
}

func (s *Article) Create(ctx context.Context, input ArticleInput) (dto.AdminArticleDetail, error) {
	normalized, err := normalizeArticleInput(input)
	if err != nil {
		return dto.AdminArticleDetail{}, err
	}

	now := s.now().UTC()
	bodyRunes, readingMinutes := readingMetrics(normalized.Body)
	article := &model.Article{
		Slug:           normalized.Slug,
		Title:          normalized.Title,
		Summary:        normalized.Summary,
		Body:           normalized.Body,
		Category:       normalized.Category,
		Tags:           normalized.Tags,
		Status:         normalized.Status,
		ReadingMinutes: readingMinutes,
		BodyRunes:      bodyRunes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if normalized.Status == model.ArticleStatusPublished {
		article.PublishedAt = &now
	}

	// 40 位随机撞一次是极小概率，撞两次基本说明随机源出了问题——与其无限重试，不如报出来。
	for attempt := 0; attempt < maxArticleIDAttempts; attempt++ {
		id, err := auth.NewShortID()
		if err != nil {
			return dto.AdminArticleDetail{}, err
		}
		article.ID = id

		err = s.repo.Create(ctx, article)
		if err == nil {
			return dto.NewAdminArticleDetail(*article), nil
		}
		if !errors.Is(err, repository.ErrIDTaken) {
			return dto.AdminArticleDetail{}, articleWriteError(err)
		}
	}

	return dto.AdminArticleDetail{}, fmt.Errorf("连续 %d 次撞上同一个主键", maxArticleIDAttempts)
}

func (s *Article) Update(ctx context.Context, id string, input ArticleInput) (dto.AdminArticleDetail, error) {
	normalized, err := normalizeArticleInput(input)
	if err != nil {
		return dto.AdminArticleDetail{}, err
	}

	current, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.AdminArticleDetail{}, ErrArticleNotFound
	}
	if err != nil {
		return dto.AdminArticleDetail{}, err
	}

	now := s.now().UTC()
	publishedAt := current.PublishedAt
	// 首次发布才盖时间戳。已发布的再存一次不动它——那是首发时间；
	// 退回草稿也不清空，重新发布时链接和先后顺序都还认得出来。
	if normalized.Status == model.ArticleStatusPublished && publishedAt == nil {
		publishedAt = &now
	}

	bodyRunes, readingMinutes := readingMetrics(normalized.Body)
	err = s.repo.Update(ctx, id, repository.ArticleUpdate{
		Slug:           normalized.Slug,
		Title:          normalized.Title,
		Summary:        normalized.Summary,
		Body:           normalized.Body,
		Category:       normalized.Category,
		Tags:           normalized.Tags,
		Status:         normalized.Status,
		PublishedAt:    publishedAt,
		ReadingMinutes: readingMinutes,
		BodyRunes:      bodyRunes,
		UpdatedAt:      now,
	})
	if err != nil {
		return dto.AdminArticleDetail{}, articleWriteError(err)
	}

	// 重新读一次，而不是把刚发出去的东西回显给前端：reading_minutes、body_runes、
	// published_at 都是服务端算的，回显等于让前端看到的和库里存的悄悄分叉。
	return s.GetForEdit(ctx, id)
}

func (s *Article) Delete(ctx context.Context, id string) error {
	return articleWriteError(s.repo.Delete(ctx, id))
}

// 把仓储的错误翻成这一层的说法，handler 就不必认识 repository 包。
func articleWriteError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return ErrArticleNotFound
	case errors.Is(err, repository.ErrSlugTaken):
		return ErrSlugTaken
	default:
		return err
	}
}

var articleSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func normalizeArticleInput(input ArticleInput) (ArticleInput, error) {
	normalized := ArticleInput{
		// 先修剪再小写，然后才校验：作者从别处粘过来的 " Go-Errors " 不该被拒。
		Slug:     strings.ToLower(strings.TrimSpace(input.Slug)),
		Title:    strings.TrimSpace(input.Title),
		Summary:  strings.TrimSpace(input.Summary),
		Body:     strings.TrimSpace(input.Body),
		Category: strings.TrimSpace(input.Category),
		Status:   strings.TrimSpace(input.Status),
	}

	switch {
	case !articleSlugPattern.MatchString(normalized.Slug),
		utf8.RuneCountInString(normalized.Slug) > articleSlugMaxRunes:
		return ArticleInput{}, ErrInvalidSlug
	case normalized.Title == "":
		return ArticleInput{}, ErrTitleRequired
	case utf8.RuneCountInString(normalized.Title) > articleTitleMaxRunes:
		return ArticleInput{}, ErrTitleTooLong
	case normalized.Summary == "":
		return ArticleInput{}, ErrSummaryRequired
	case utf8.RuneCountInString(normalized.Summary) > articleSummaryMaxRunes:
		return ArticleInput{}, ErrSummaryTooLong
	case normalized.Body == "":
		return ArticleInput{}, ErrBodyRequired
	case utf8.RuneCountInString(normalized.Body) > articleBodyMaxRunes:
		return ArticleInput{}, ErrBodyTooLong
	case !IsArticleCategory(normalized.Category):
		return ArticleInput{}, ErrInvalidCategory
	case normalized.Status != model.ArticleStatusDraft && normalized.Status != model.ArticleStatusPublished:
		return ArticleInput{}, ErrInvalidStatus
	}

	// 标签去掉空白项、按大小写不敏感去重（Go 和 go 并排显示是看不出区别的），
	// 保留第一次出现的写法。
	seen := make(map[string]struct{}, len(input.Tags))
	tags := make([]string, 0, len(input.Tags))
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > articleTagMaxRunes {
			return ArticleInput{}, ErrTagTooLong
		}
		key := strings.ToLower(tag)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > maxArticleTags {
		return ArticleInput{}, ErrTooManyTags
	}
	normalized.Tags = tags

	return normalized, nil
}

// 两个数同源：都由同一份正文的 rune 数算出来，一起落库——分开算迟早有一个忘了跟着改。
// 按含 markdown 标记的字符数估，不去解析语法：剥掉代码块和链接会准一点，
// 但会把一个纯函数变成半个解析器，而它只是个给读者看的参考值。
func readingMetrics(body string) (runes, minutes int) {
	runes = utf8.RuneCountInString(body)
	minutes = (runes + readingRunesPerMinute - 1) / readingRunesPerMinute
	if minutes < 1 {
		minutes = 1
	}
	return runes, minutes
}
