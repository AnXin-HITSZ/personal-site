package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

var testCategoryNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// 各分类的「最近一篇」各是一个不同的日子：接线接反了（比如每条都拿到同一个值）
// 能当场看见。
var (
	backendLatestAt  = time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	aiLatestAt       = time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC)
	frontendLatestAt = time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
)

// 分类 id 一律是建分类时现取的 8 位随机码，种子行也照那个形状写；好几处断言都要
// 引用它们，写成一串常量免得看花眼。
const (
	backendCat  = "b7k2m9pq"
	frontendCat = "f8n3s5t7"
	aiCat       = "a9w2y4z6"
	notesCat    = "n6h4j2m8"
)

type fakeCategoryStore struct {
	rows []repository.CategoryRow

	created []*model.Category
	renamed []string
	ordered [][]string
	deleted []string
	moved   []string

	// 前 idCollisions 次 Create 报主键撞了，用来测重试。
	idCollisions int
	createCalls  int
	err          error
}

func (f *fakeCategoryStore) ListWithUsage(context.Context) ([]repository.CategoryRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func (f *fakeCategoryStore) Create(_ context.Context, category *model.Category, _ int) error {
	f.createCalls++
	if f.createCalls <= f.idCollisions {
		return repository.ErrIDTaken
	}
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, category)
	return nil
}

func (f *fakeCategoryStore) Rename(_ context.Context, id, name string, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.renamed = append(f.renamed, id+"="+name)
	return nil
}

func (f *fakeCategoryStore) Reorder(_ context.Context, ids []string, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.ordered = append(f.ordered, ids)
	return nil
}

func (f *fakeCategoryStore) Delete(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeCategoryStore) DeleteAndMove(_ context.Context, id, moveTo string, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.moved = append(f.moved, id+"→"+moveTo)
	return nil
}

func newCategoryService(store *fakeCategoryStore) *Category {
	service := NewCategory(store)
	service.now = func() time.Time { return testCategoryNow }
	return service
}

func seededCategories() []repository.CategoryRow {
	return []repository.CategoryRow{
		{ID: backendCat, Name: "后端开发", Total: 4, Drafts: 1, Published: 3, LatestPublishedAt: &backendLatestAt},
		{ID: frontendCat, Name: "前端实践", Total: 2, Drafts: 0, Published: 2, LatestPublishedAt: &frontendLatestAt},
		{ID: aiCat, Name: "AI 探索", Total: 2, Drafts: 1, Published: 1, LatestPublishedAt: &aiLatestAt},
		{ID: notesCat, Name: "学习随笔", Total: 1, Drafts: 1, Published: 0},
		{ID: "abcd1234", Name: "读书笔记", Total: 0, Drafts: 0, Published: 0},
	}
}

// 读者那一行只列有已发布文章的分类。零篇（读书笔记）和「全是草稿」（学习随笔）
// 在读者那边是一回事：点进去什么都没有，那个按钮就只是占位。篇数与最近时间也
// 都只数已发布的——后端开发 4 篇里 1 篇是草稿，读者看到的是 3 篇。
func TestListPublishedHidesCategoriesWithoutPublishedArticles(t *testing.T) {
	service := newCategoryService(&fakeCategoryStore{rows: seededCategories()})

	list, err := service.ListPublished(context.Background())
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	want := []struct {
		id     string
		name   string
		count  int
		latest time.Time
	}{
		{backendCat, "后端开发", 3, backendLatestAt},
		{frontendCat, "前端实践", 2, frontendLatestAt},
		{aiCat, "AI 探索", 1, aiLatestAt},
	}
	if len(list.Items) != len(want) {
		t.Fatalf("应有 %d 个分类（顺序照 sort_order），实际 %+v", len(want), list.Items)
	}
	for i, w := range want {
		item := list.Items[i]
		if item.ID != w.id || item.Name != w.name || item.ArticleCount != w.count ||
			item.LatestPublishedAt == nil || !item.LatestPublishedAt.Equal(w.latest) {
			t.Errorf("第 %d 条应为 %s「%s、%d 篇、最近 %v」，实际 %+v", i+1, w.id, w.name, w.count, w.latest, item)
		}
	}
}

func TestListPublishedRejectsDraftOnlyCategory(t *testing.T) {
	notesLatest := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	service := newCategoryService(&fakeCategoryStore{rows: []repository.CategoryRow{
		{ID: backendCat, Name: "后端开发", Total: 3, Drafts: 3},
		{ID: notesCat, Name: "学习随笔", Total: 2, Drafts: 1, Published: 1, LatestPublishedAt: &notesLatest},
	}})

	list, err := service.ListPublished(context.Background())
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	if len(list.Items) != 1 || list.Items[0].ID != notesCat {
		t.Fatalf("只有 %s 有一篇发出去的，实际 %+v", notesCat, list.Items)
	}
	if item := list.Items[0]; item.ArticleCount != 1 || item.LatestPublishedAt == nil || !item.LatestPublishedAt.Equal(notesLatest) {
		t.Errorf("学习随笔应带上「1 篇、最近 %v」，实际 %+v", notesLatest, item)
	}
}

func TestListPublishedReturnsEmptySliceNotNull(t *testing.T) {
	service := newCategoryService(&fakeCategoryStore{})

	list, err := service.ListPublished(context.Background())
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	// nil 切片序列化成 null，前端拿到 null 再 .map 就炸了。
	if list.Items == nil {
		t.Error("一个分类都没有时也该是空数组，不是 null")
	}
}

func TestListAdminReportsBothCounts(t *testing.T) {
	service := newCategoryService(&fakeCategoryStore{rows: seededCategories()})

	list, err := service.ListAdmin(context.Background())
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	if len(list.Items) != 5 {
		t.Fatalf("应有 5 个分类（零篇的那个也在），实际 %d", len(list.Items))
	}
	// 写作那一页看得见零篇的分类——正因为看得见，才删得掉它。
	last := list.Items[4]
	if last.ID != "abcd1234" || last.ArticleCount != 0 || last.DraftCount != 0 {
		t.Errorf("最后一个应是零篇的读书笔记，实际 %+v", last)
	}
	if list.Items[0].ArticleCount != 4 || list.Items[0].DraftCount != 1 {
		t.Errorf("后端开发应是 4 篇其中 1 篇草稿，实际 %+v", list.Items[0])
	}
}

func TestCreateNormalizesName(t *testing.T) {
	store := &fakeCategoryStore{}
	service := newCategoryService(store)

	category, err := service.Create(context.Background(), "  读书笔记  ")
	if err != nil {
		t.Fatalf("应被接受，实际报错：%v", err)
	}

	if category.Name != "读书笔记" {
		t.Errorf("名字应去掉首尾空白，实际 %q", category.Name)
	}
	if len(store.created) != 1 {
		t.Fatalf("应落库一次，实际 %d 次", len(store.created))
	}
	// 新分类下面不可能有文章，两个计数不必再查一次库。
	if category.ArticleCount != 0 || category.DraftCount != 0 {
		t.Errorf("新分类的两个计数应为 0，实际 %+v", category)
	}
	if got := store.created[0].CreatedAt; !got.Equal(testCategoryNow) {
		t.Errorf("创建时刻应是固定的 UTC 时刻，实际 %v", got)
	}
}

func TestCreateRejectsBadName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{name: "空", in: "", want: ErrCategoryNameRequired},
		{name: "只有空格", in: "   ", want: ErrCategoryNameRequired},
		{name: "换行", in: "\n\t", want: ErrCategoryNameRequired},
		{name: "太长", in: strings.Repeat("字", categoryNameMaxRunes+1), want: ErrCategoryNameTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCategoryStore{}
			service := newCategoryService(store)

			if _, err := service.Create(context.Background(), tc.in); !errors.Is(err, tc.want) {
				t.Errorf("应为 %v，实际 %v", tc.want, err)
			}
			if len(store.created) != 0 {
				t.Error("名字不合法时不该落库")
			}
		})
	}
}

func TestCreateAcceptsNameAtTheLimit(t *testing.T) {
	service := newCategoryService(&fakeCategoryStore{})

	if _, err := service.Create(context.Background(), strings.Repeat("字", categoryNameMaxRunes)); err != nil {
		t.Errorf("正好 %d 个字应被接受，实际报错：%v", categoryNameMaxRunes, err)
	}
}

func TestCreateCategoryRetriesOnIDCollision(t *testing.T) {
	store := &fakeCategoryStore{idCollisions: 1}
	service := newCategoryService(store)

	if _, err := service.Create(context.Background(), "读书笔记"); err != nil {
		t.Fatalf("撞一次主键应重试，实际报错：%v", err)
	}
	if store.createCalls != 2 {
		t.Errorf("应写两次（第一次撞、第二次成），实际 %d 次", store.createCalls)
	}
}

func TestCreateCategoryGivesUpOnRepeatedIDCollision(t *testing.T) {
	store := &fakeCategoryStore{idCollisions: maxCategoryIDAttempts}
	service := newCategoryService(store)

	if _, err := service.Create(context.Background(), "读书笔记"); err == nil {
		t.Fatal("连续撞满次数之后应报错，而不是继续重试")
	}
	if store.createCalls != maxCategoryIDAttempts {
		t.Errorf("应只试 %d 次，实际 %d 次", maxCategoryIDAttempts, store.createCalls)
	}
}

func TestCreateReportsTakenName(t *testing.T) {
	store := &fakeCategoryStore{err: repository.ErrCategoryNameTaken}
	service := newCategoryService(store)

	if _, err := service.Create(context.Background(), "前端实践"); !errors.Is(err, ErrCategoryNameTaken) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryNameTaken, err)
	}
}

func TestCreateReportsTooMany(t *testing.T) {
	store := &fakeCategoryStore{err: repository.ErrTooManyCategories}
	service := newCategoryService(store)

	if _, err := service.Create(context.Background(), "读书笔记"); !errors.Is(err, ErrTooManyCategories) {
		t.Errorf("应为 %v，实际 %v", ErrTooManyCategories, err)
	}
}

func TestRenameKeepsTheCounts(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	category, err := service.Rename(context.Background(), backendCat, "  服务端  ")
	if err != nil {
		t.Fatalf("改名失败：%v", err)
	}

	if category.Name != "服务端" {
		t.Errorf("应回显新的名字，实际 %q", category.Name)
	}
	// 改名字不动任何一篇文章，所以刚读到的数就是改完之后的数。
	if category.ArticleCount != 4 || category.DraftCount != 1 {
		t.Errorf("两个计数应原样带回来，实际 %+v", category)
	}
	if len(store.renamed) != 1 || store.renamed[0] != backendCat+"=服务端" {
		t.Errorf("应按修剪后的名字落库，实际 %v", store.renamed)
	}
}

func TestRenameMissingCategory(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if _, err := service.Rename(context.Background(), "zzzz9999", "服务端"); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryNotFound, err)
	}
	if len(store.renamed) != 0 {
		t.Error("分类不存在时不该写任何东西")
	}
}

func TestRenameRejectsBadNameBeforeLookingItUp(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if _, err := service.Rename(context.Background(), backendCat, "  "); !errors.Is(err, ErrCategoryNameRequired) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryNameRequired, err)
	}
}

func TestReorderRejectsMalformedIDsBeforeWriting(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	err := service.Reorder(context.Background(), []string{"abcd1234", "ai"})
	if !errors.Is(err, ErrCategoryOrderMismatch) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryOrderMismatch, err)
	}
	if len(store.ordered) != 0 {
		t.Error("顺序里有一段不像 id，整份都不该提交下去")
	}
}

func TestReorderPassesTheWholeListThrough(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	ids := []string{"abcd1234", backendCat, frontendCat, aiCat, notesCat}
	if err := service.Reorder(context.Background(), ids); err != nil {
		t.Fatalf("排序失败：%v", err)
	}

	if len(store.ordered) != 1 || strings.Join(store.ordered[0], ",") != strings.Join(ids, ",") {
		t.Errorf("应把整份顺序原样交下去，实际 %v", store.ordered)
	}
}

func TestDeleteEmptyCategory(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), "abcd1234", ""); err != nil {
		t.Fatalf("删空分类失败：%v", err)
	}

	if len(store.deleted) != 1 || store.deleted[0] != "abcd1234" {
		t.Errorf("应直接删掉它，实际 deleted=%v moved=%v", store.deleted, store.moved)
	}
}

// 有文章又不给去处就不动手：这不是前端拦的，外键也拦着，这里只是把话说在前面。
func TestDeleteRefusesCategoryInUseWithoutADestination(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), backendCat, ""); !errors.Is(err, ErrCategoryInUse) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryInUse, err)
	}
	if len(store.deleted) != 0 || len(store.moved) != 0 {
		t.Error("没有去处时不该动任何一行")
	}
}

// 有一篇草稿也算有文章：外键拦的是行，不是状态。
func TestDeleteRefusesDraftOnlyCategoryWithoutADestination(t *testing.T) {
	store := &fakeCategoryStore{rows: []repository.CategoryRow{{ID: notesCat, Name: "学习随笔", Total: 1, Drafts: 1}}}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), notesCat, ""); !errors.Is(err, ErrCategoryInUse) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryInUse, err)
	}
}

func TestDeleteMovesTheArticlesFirst(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), backendCat, notesCat); err != nil {
		t.Fatalf("挪走再删失败：%v", err)
	}

	if len(store.moved) != 1 || store.moved[0] != backendCat+"→"+notesCat {
		t.Errorf("应交下去「%s 挪到 %s」，实际 %v", backendCat, notesCat, store.moved)
	}
	// 两件事是一个事务里的一件事，不分成两次调用。
	if len(store.deleted) != 0 {
		t.Errorf("挪走那一支里不该再单独调一次删除：%v", store.deleted)
	}
}

func TestDeleteRefusesMovingToItself(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), backendCat, backendCat); !errors.Is(err, ErrCategoryMoveToSelf) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryMoveToSelf, err)
	}
	if len(store.moved) != 0 {
		t.Error("去处是它自己时不该动手——那样文章会指着一个刚被删掉的分类")
	}
}

func TestDeleteRejectsMalformedDestination(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), backendCat, "读书笔记"); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryNotFound, err)
	}
	if len(store.moved) != 0 {
		t.Error("去处不像 id 时不该提交下去")
	}
}

func TestDeleteMissingCategory(t *testing.T) {
	store := &fakeCategoryStore{rows: seededCategories()}
	service := newCategoryService(store)

	if err := service.Delete(context.Background(), "zzzz9999", ""); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrCategoryNotFound, err)
	}
}

func TestIsCategoryID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// 定长 8 位。内置短代号清掉之后，旧的 backend／ai 都不再放行。
		{in: "abcd1234", want: true},
		{in: "zzzz9999", want: true},
		{in: "01234567", want: true},
		{in: "", want: false},
		{in: "ai", want: false},
		{in: "backend", want: false},
		{in: "Backend", want: false},
		{in: "backend-1", want: false},
		{in: "后端", want: false},
		{in: "abcd123", want: false},
		{in: "abcd12345", want: false},
		{in: "0123456789abcdef", want: false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.in), func(t *testing.T) {
			if got := IsCategoryID(tc.in); got != tc.want {
				t.Errorf("IsCategoryID(%q) 应为 %v", tc.in, tc.want)
			}
		})
	}
}

func TestCategoryWriteErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{name: "记录不存在", in: repository.ErrNotFound, want: ErrCategoryNotFound},
		{name: "名字撞了", in: repository.ErrCategoryNameTaken, want: ErrCategoryNameTaken},
		{name: "下面还有文章", in: repository.ErrCategoryInUse, want: ErrCategoryInUse},
		{name: "到上限了", in: repository.ErrTooManyCategories, want: ErrTooManyCategories},
		{name: "顺序对不上", in: repository.ErrOrderMismatch, want: ErrCategoryOrderMismatch},
		{name: "分类不存在", in: repository.ErrCategoryNotFound, want: ErrCategoryNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := categoryWriteError(tc.in); !errors.Is(got, tc.want) {
				t.Errorf("应为 %v，实际 %v", tc.want, got)
			}
		})
	}
}
