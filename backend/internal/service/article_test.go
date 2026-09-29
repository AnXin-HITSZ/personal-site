package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

// 固定一个时刻，好让「发布时盖的时间戳」能精确断言。
var testArticleNow = time.Date(2026, 9, 28, 8, 30, 0, 0, time.UTC)

type fakeArticleStore struct {
	articles map[string]*model.Article
	created  []*model.Article
	updates  map[string]repository.ArticleUpdate
	deleted  []string

	// 只让写路径失败。读路径跟着一起坏的话，「slug 撞车翻成哪个错误」这类断言
	// 会从 GetByID 上先弹回来，测的就不是它想测的东西了。
	writeErr error
	readErr  error

	// 让前 N 次 Create 撞主键，用来测重试。
	idTakenTimes int
	lists        []repository.AdminArticleFilter
}

func newFakeArticleStore() *fakeArticleStore {
	return &fakeArticleStore{
		articles: map[string]*model.Article{},
		updates:  map[string]repository.ArticleUpdate{},
	}
}

func (f *fakeArticleStore) seed(article model.Article) {
	copied := article
	f.articles[copied.ID] = &copied
}

func (f *fakeArticleStore) Create(_ context.Context, article *model.Article) error {
	if f.idTakenTimes > 0 {
		f.idTakenTimes--
		return repository.ErrIDTaken
	}
	if f.writeErr != nil {
		return f.writeErr
	}
	copied := *article
	copied.Tags = append([]string(nil), article.Tags...)
	f.articles[copied.ID] = &copied
	f.created = append(f.created, &copied)
	return nil
}

func (f *fakeArticleStore) Update(_ context.Context, id string, update repository.ArticleUpdate) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	article, ok := f.articles[id]
	if !ok {
		return repository.ErrNotFound
	}
	f.updates[id] = update
	article.Slug = update.Slug
	article.Title = update.Title
	article.Status = update.Status
	article.PublishedAt = update.PublishedAt
	article.ReadingMinutes = update.ReadingMinutes
	article.BodyRunes = update.BodyRunes
	article.UpdatedAt = update.UpdatedAt
	return nil
}

func (f *fakeArticleStore) Delete(_ context.Context, id string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if _, ok := f.articles[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.articles, id)
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeArticleStore) GetByID(_ context.Context, id string) (*model.Article, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	article, ok := f.articles[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return article, nil
}

func (f *fakeArticleStore) GetPublishedByID(_ context.Context, id string) (*model.Article, error) {
	article, err := f.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if article.Status != model.ArticleStatusPublished {
		return nil, repository.ErrNotFound
	}
	return article, nil
}

func (f *fakeArticleStore) ListPublished(context.Context, string, string, int, int) ([]model.Article, int, error) {
	return nil, 0, errors.New("这个测试用不到")
}

func (f *fakeArticleStore) ListAdmin(_ context.Context, filter repository.AdminArticleFilter, limit, offset int) ([]model.Article, int, error) {
	if f.readErr != nil {
		return nil, 0, f.readErr
	}
	f.lists = append(f.lists, filter)

	all := make([]model.Article, 0, len(f.articles))
	for _, article := range f.articles {
		if filter.Status != "" && article.Status != filter.Status {
			continue
		}
		all = append(all, *article)
	}
	if offset >= len(all) {
		return []model.Article{}, len(all), nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], len(all), nil
}

func newArticleService(store *fakeArticleStore) *Article {
	service := NewArticle(store)
	service.now = func() time.Time { return testArticleNow }
	return service
}

func validInput() ArticleInput {
	return ArticleInput{
		Slug:     "building-my-blog",
		Title:    "把博客搭起来",
		Summary:  "从零开始的第一步。",
		Body:     strings.Repeat("正", 400),
		Category: "backend",
		Tags:     []string{"go", "gin"},
		Status:   model.ArticleStatusDraft,
	}
}

func TestReadingMetrics(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantRunes   int
		wantMinutes int
	}{
		{name: "空正文也至少一分钟", body: "", wantRunes: 0, wantMinutes: 1},
		{name: "一个字", body: "好", wantRunes: 1, wantMinutes: 1},
		{name: "正好一个单位", body: strings.Repeat("a", readingRunesPerMinute), wantRunes: readingRunesPerMinute, wantMinutes: 1},
		{name: "多一个字就进位", body: strings.Repeat("a", readingRunesPerMinute+1), wantRunes: readingRunesPerMinute + 1, wantMinutes: 2},
		{name: "两个单位", body: strings.Repeat("a", readingRunesPerMinute*2), wantRunes: readingRunesPerMinute * 2, wantMinutes: 2},
		{name: "中文按字数算，不是字节数", body: strings.Repeat("好", readingRunesPerMinute*2), wantRunes: readingRunesPerMinute * 2, wantMinutes: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runes, minutes := readingMetrics(tc.body)
			if runes != tc.wantRunes || minutes != tc.wantMinutes {
				t.Errorf("应为 %d 字／%d 分钟，实际 %d 字／%d 分钟", tc.wantRunes, tc.wantMinutes, runes, minutes)
			}
		})
	}
}

func TestNormalizeArticleInputRejects(t *testing.T) {
	cases := []struct {
		name  string
		alter func(*ArticleInput)
		want  error
	}{
		{name: "空 slug", alter: func(in *ArticleInput) { in.Slug = "" }, want: ErrInvalidSlug},
		{name: "slug 带大写之外的空格在中间", alter: func(in *ArticleInput) { in.Slug = "my blog" }, want: ErrInvalidSlug},
		{name: "slug 以连字符开头", alter: func(in *ArticleInput) { in.Slug = "-my-blog" }, want: ErrInvalidSlug},
		{name: "slug 以连字符结尾", alter: func(in *ArticleInput) { in.Slug = "my-blog-" }, want: ErrInvalidSlug},
		{name: "slug 连用两个连字符", alter: func(in *ArticleInput) { in.Slug = "my--blog" }, want: ErrInvalidSlug},
		{name: "slug 带下划线", alter: func(in *ArticleInput) { in.Slug = "my_blog" }, want: ErrInvalidSlug},
		{name: "slug 太长", alter: func(in *ArticleInput) { in.Slug = strings.Repeat("a", articleSlugMaxRunes+1) }, want: ErrInvalidSlug},
		{name: "标题为空", alter: func(in *ArticleInput) { in.Title = "   " }, want: ErrTitleRequired},
		{name: "标题太长", alter: func(in *ArticleInput) { in.Title = strings.Repeat("标", articleTitleMaxRunes+1) }, want: ErrTitleTooLong},
		{name: "摘要为空", alter: func(in *ArticleInput) { in.Summary = "" }, want: ErrSummaryRequired},
		{name: "摘要太长", alter: func(in *ArticleInput) { in.Summary = strings.Repeat("摘", articleSummaryMaxRunes+1) }, want: ErrSummaryTooLong},
		{name: "正文为空", alter: func(in *ArticleInput) { in.Body = "\n\n" }, want: ErrBodyRequired},
		{name: "正文太长", alter: func(in *ArticleInput) { in.Body = strings.Repeat("正", articleBodyMaxRunes+1) }, want: ErrBodyTooLong},
		{name: "未知分类", alter: func(in *ArticleInput) { in.Category = "life" }, want: ErrInvalidCategory},
		{name: "分类为空", alter: func(in *ArticleInput) { in.Category = "" }, want: ErrInvalidCategory},
		{name: "未知状态", alter: func(in *ArticleInput) { in.Status = "archived" }, want: ErrInvalidStatus},
		{name: "状态为空", alter: func(in *ArticleInput) { in.Status = "" }, want: ErrInvalidStatus},
		{name: "标签太多", alter: func(in *ArticleInput) {
			in.Tags = nil
			for i := 0; i <= maxArticleTags; i++ {
				in.Tags = append(in.Tags, string(rune('a'+i)))
			}
		}, want: ErrTooManyTags},
		{name: "单个标签太长", alter: func(in *ArticleInput) { in.Tags = []string{strings.Repeat("x", articleTagMaxRunes+1)} }, want: ErrTagTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validInput()
			tc.alter(&input)
			if _, err := normalizeArticleInput(input); !errors.Is(err, tc.want) {
				t.Errorf("应为 %v，实际 %v", tc.want, err)
			}
		})
	}
}

func TestNormalizeArticleInputAccepts(t *testing.T) {
	input := validInput()
	input.Slug = "  Go-Errors  "
	input.Title = "  标题  "
	input.Tags = []string{" Go ", "go", "", "  ", "Gin"}

	normalized, err := normalizeArticleInput(input)
	if err != nil {
		t.Fatalf("应被接受，实际报错：%v", err)
	}
	if normalized.Slug != "go-errors" {
		t.Errorf("slug 应先修剪再小写，实际 %q", normalized.Slug)
	}
	if normalized.Title != "标题" {
		t.Errorf("标题应去掉首尾空白，实际 %q", normalized.Title)
	}
	want := []string{"Go", "Gin"}
	if len(normalized.Tags) != len(want) {
		t.Fatalf("标签应去空并按大小写不敏感去重，实际 %v", normalized.Tags)
	}
	for i := range want {
		if normalized.Tags[i] != want[i] {
			t.Errorf("第 %d 个标签应为 %q，实际 %q（保留第一次出现的写法）", i, want[i], normalized.Tags[i])
		}
	}
}

func TestNormalizeArticleInputAlwaysReturnsTagsSlice(t *testing.T) {
	input := validInput()
	input.Tags = nil

	normalized, err := normalizeArticleInput(input)
	if err != nil {
		t.Fatalf("没有标签应当合法：%v", err)
	}
	if normalized.Tags == nil {
		t.Error("标签应为空切片而不是 nil——nil 会序列化成 null")
	}
}

func TestCreateDraftHasNoPublishedAt(t *testing.T) {
	store := newFakeArticleStore()
	detail, err := newArticleService(store).Create(context.Background(), validInput())
	if err != nil {
		t.Fatalf("创建失败：%v", err)
	}

	if detail.PublishedAt != nil {
		t.Errorf("草稿不该有发布时刻，实际 %v", detail.PublishedAt)
	}
	if !auth.IsShortID(detail.ID) {
		t.Errorf("主键应是 8 位短 id，实际 %q", detail.ID)
	}
	// validInput 的正文是 400 个字：不够两个阅读单位，但字数就是 400。
	if detail.BodyRunes != 400 {
		t.Errorf("正文字数应由服务端数，实际 %d", detail.BodyRunes)
	}
	if detail.ReadingMinutes != 2 {
		t.Errorf("阅读时间应由服务端按正文算，实际 %d", detail.ReadingMinutes)
	}
}

func TestCreatePublishedStampsNow(t *testing.T) {
	store := newFakeArticleStore()
	input := validInput()
	input.Status = model.ArticleStatusPublished

	detail, err := newArticleService(store).Create(context.Background(), input)
	if err != nil {
		t.Fatalf("创建失败：%v", err)
	}

	if detail.PublishedAt == nil {
		t.Fatal("已发布的文章必须有发布时刻")
	}
	if !detail.PublishedAt.Equal(testArticleNow) {
		t.Errorf("发布时刻应为 %v，实际 %v", testArticleNow, *detail.PublishedAt)
	}
}

func TestCreateRetriesOnIDCollision(t *testing.T) {
	store := newFakeArticleStore()
	store.idTakenTimes = 2

	if _, err := newArticleService(store).Create(context.Background(), validInput()); err != nil {
		t.Fatalf("撞两次主键后应重试成功：%v", err)
	}
	if len(store.created) != 1 {
		t.Errorf("应只留下一条记录，实际 %d 条", len(store.created))
	}
}

func TestCreateGivesUpOnRepeatedIDCollision(t *testing.T) {
	store := newFakeArticleStore()
	store.idTakenTimes = maxArticleIDAttempts

	if _, err := newArticleService(store).Create(context.Background(), validInput()); err == nil {
		t.Error("连续撞满次数后应报错，而不是无限重试")
	}
}

func TestCreateRejectsDuplicateSlug(t *testing.T) {
	store := newFakeArticleStore()
	store.writeErr = repository.ErrSlugTaken

	_, err := newArticleService(store).Create(context.Background(), validInput())
	if !errors.Is(err, ErrSlugTaken) {
		t.Errorf("slug 撞车应翻成 %v，实际 %v", ErrSlugTaken, err)
	}
}

func TestCreateRejectsUnnormalizedInput(t *testing.T) {
	store := newFakeArticleStore()
	input := validInput()
	input.Slug = "My Blog"

	if _, err := newArticleService(store).Create(context.Background(), input); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("应为 %v，实际 %v", ErrInvalidSlug, err)
	}
	if len(store.created) != 0 {
		t.Error("校验没过就不该写库")
	}
}

func TestUpdatePublishesDraft(t *testing.T) {
	store := newFakeArticleStore()
	store.seed(model.Article{ID: "a7k2m9pq", Status: model.ArticleStatusDraft})

	input := validInput()
	input.Status = model.ArticleStatusPublished

	detail, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", input)
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}
	if detail.PublishedAt == nil || !detail.PublishedAt.Equal(testArticleNow) {
		t.Errorf("首次发布应盖上此刻的时间戳，实际 %v", detail.PublishedAt)
	}
}

func TestUpdateKeepsOriginalPublishedAt(t *testing.T) {
	firstPublished := testArticleNow.AddDate(0, -3, 0)
	store := newFakeArticleStore()
	store.seed(model.Article{
		ID:          "a7k2m9pq",
		Status:      model.ArticleStatusPublished,
		PublishedAt: &firstPublished,
	})

	input := validInput()
	input.Status = model.ArticleStatusPublished

	detail, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", input)
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}
	if detail.PublishedAt == nil || !detail.PublishedAt.Equal(firstPublished) {
		t.Errorf("再次保存已发布的文章不该动首发时间，应为 %v，实际 %v", firstPublished, detail.PublishedAt)
	}
}

func TestUpdateToDraftKeepsPublishedAt(t *testing.T) {
	firstPublished := testArticleNow.AddDate(0, -3, 0)
	store := newFakeArticleStore()
	store.seed(model.Article{
		ID:          "a7k2m9pq",
		Status:      model.ArticleStatusPublished,
		PublishedAt: &firstPublished,
	})

	// 退回草稿，published_at 保留：重新发布时链接的先后顺序还认得出来。
	input := validInput()
	input.Status = model.ArticleStatusDraft

	detail, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", input)
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}
	if detail.PublishedAt == nil || !detail.PublishedAt.Equal(firstPublished) {
		t.Errorf("退回草稿不该清掉发布时刻，应为 %v，实际 %v", firstPublished, detail.PublishedAt)
	}
}

func TestUpdateWritesUpdatedAtInUTC(t *testing.T) {
	store := newFakeArticleStore()
	store.seed(model.Article{ID: "a7k2m9pq", Status: model.ArticleStatusDraft})

	if _, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", validInput()); err != nil {
		t.Fatalf("更新失败：%v", err)
	}

	update := store.updates["a7k2m9pq"]
	if !update.UpdatedAt.Equal(testArticleNow) || update.UpdatedAt.Location() != time.UTC {
		t.Errorf("updated_at 应是 UTC 的 %v，实际 %v", testArticleNow, update.UpdatedAt)
	}
	if update.BodyRunes != 400 || update.ReadingMinutes != 2 {
		t.Error("字数与阅读时间都该由服务端重算，不采信请求里的值")
	}
}

func TestUpdateRecomputesBothMetrics(t *testing.T) {
	store := newFakeArticleStore()
	// 库里这两个数还是上一稿的，正文换了就得两个一起换。
	store.seed(model.Article{ID: "a7k2m9pq", Status: model.ArticleStatusDraft, ReadingMinutes: 99, BodyRunes: 999})

	detail, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", validInput())
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}

	if detail.BodyRunes != 400 || detail.ReadingMinutes != 2 {
		t.Errorf("两个数都该按新正文重算，实际 %d 字／%d 分钟", detail.BodyRunes, detail.ReadingMinutes)
	}
}

func TestUpdateMissingArticle(t *testing.T) {
	store := newFakeArticleStore()

	_, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", validInput())
	if !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrArticleNotFound, err)
	}
}

func TestUpdateRejectsDuplicateSlug(t *testing.T) {
	store := newFakeArticleStore()
	store.seed(model.Article{ID: "a7k2m9pq"})
	store.writeErr = repository.ErrSlugTaken

	_, err := newArticleService(store).Update(context.Background(), "a7k2m9pq", validInput())
	if !errors.Is(err, ErrSlugTaken) {
		t.Errorf("slug 撞车应翻成 %v，实际 %v", ErrSlugTaken, err)
	}
}

func TestDeleteMissingArticle(t *testing.T) {
	store := newFakeArticleStore()

	if err := newArticleService(store).Delete(context.Background(), "a7k2m9pq"); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrArticleNotFound, err)
	}
}

func TestGetPublishedHidesDrafts(t *testing.T) {
	store := newFakeArticleStore()
	store.seed(model.Article{ID: "a7k2m9pq", Status: model.ArticleStatusDraft})

	_, err := newArticleService(store).GetPublished(context.Background(), "a7k2m9pq")
	if !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("草稿不该从公开接口取到，实际 %v", err)
	}
}

func TestGetForEditSeesDrafts(t *testing.T) {
	store := newFakeArticleStore()
	store.seed(model.Article{ID: "a7k2m9pq", Status: model.ArticleStatusDraft, Body: "正文"})

	detail, err := newArticleService(store).GetForEdit(context.Background(), "a7k2m9pq")
	if err != nil {
		t.Fatalf("后台应能取到草稿：%v", err)
	}
	if detail.Body != "正文" {
		t.Errorf("编辑页要拿到正文，实际 %q", detail.Body)
	}
}

func TestListAdminBuildsPagination(t *testing.T) {
	store := newFakeArticleStore()
	for _, id := range []string{"a7k2m9pq", "b7k2m9pq", "c7k2m9pq"} {
		store.seed(model.Article{ID: id, Status: model.ArticleStatusDraft})
	}

	result, err := newArticleService(store).ListAdmin(context.Background(), dto.AdminArticleListQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("列表失败：%v", err)
	}
	if result.Pagination.Total != 3 || result.Pagination.TotalPages != 2 {
		t.Errorf("总数/页数应为 3/2，实际 %d/%d", result.Pagination.Total, result.Pagination.TotalPages)
	}
	if len(result.Items) != 1 {
		t.Errorf("第二页应剩 1 条，实际 %d 条", len(result.Items))
	}
	if result.Items[0].Status != model.ArticleStatusDraft || result.Items[0].PublishedAt != nil {
		t.Error("草稿的 publishedAt 应为 null，而不是让构造失败")
	}
}

func TestListAdminRejectsBadPagination(t *testing.T) {
	store := newFakeArticleStore()

	_, err := newArticleService(store).ListAdmin(context.Background(), dto.AdminArticleListQuery{Page: 0, PageSize: 20})
	if err == nil {
		t.Error("页码为 0 应该报错，不该当成第一页")
	}
	if len(store.lists) != 0 {
		t.Error("参数不合法就不该查库")
	}
}
