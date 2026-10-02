package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

type articleFixture struct {
	repo *Article
	rec  *recorder
	conn *dryRunConn
}

func newArticleFixture(t *testing.T) articleFixture {
	t.Helper()
	db, rec, conn := newDryRunDB(t)
	return articleFixture{repo: NewArticle(db), rec: rec, conn: conn}
}

// DryRun 下 Updates 不会真的执行，但 GORM 仍然会把 map 翻成语句，
// 所以这里能看到真正要发出去的那条 SQL——包括每个值长什么样。
func TestUpdateSerializesTagsAsJSON(t *testing.T) {
	f := newArticleFixture(t)
	f.rec.setRowCount(1)
	now := time.Date(2026, 9, 28, 8, 30, 0, 0, time.UTC)

	err := f.repo.Update(context.Background(), "a7k2m9pq", ArticleUpdate{
		Slug:           "building-my-blog",
		Title:          "标题",
		Summary:        "摘要",
		Body:           "正文",
		Category:       backendCat,
		Tags:           []string{"go", "gin"},
		Status:         "draft",
		ReadingMinutes: 3,
		BodyRunes:      400,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}

	// map 更新不会替 tags 跑 serializer，直接放 []string 进去驱动会报
	// 「不支持的类型」。所以断言的是 JSON 文本，而不是 Go 切片。
	statement := f.rec.expect(t, "UPDATE `articles`", "tags")
	if !strings.Contains(statement, `'["go","gin"]'`) {
		t.Errorf("tags 应被序列化成 JSON 文本：\n%s", statement)
	}
	if !strings.Contains(statement, "2026-09-28 08:30:00") {
		t.Errorf("updated_at 应显式带上 UTC 的时刻，而不是留给 GORM 自己填：\n%s", statement)
	}
	if !strings.Contains(statement, "'draft'") {
		t.Errorf("status 没有出现在更新里：\n%s", statement)
	}
	if !strings.Contains(statement, "`body_runes`=400") {
		t.Errorf("正文字数没有出现在更新里：\n%s", statement)
	}
	if !strings.Contains(statement, "WHERE id = 'a7k2m9pq'") {
		t.Errorf("更新应按 id 定位：\n%s", statement)
	}
}

func TestUpdateWritesNullPublishedAtForDraft(t *testing.T) {
	f := newArticleFixture(t)
	f.rec.setRowCount(1)

	err := f.repo.Update(context.Background(), "a7k2m9pq", ArticleUpdate{
		Slug:      "draft-post",
		Status:    "draft",
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("更新失败：%v", err)
	}

	// 草稿没有首发时间。写成零值时间的话，公开列表按 published_at 排序时
	// 它就会挤到最前面。
	f.rec.expect(t, "UPDATE `articles`", "published_at`=NULL")
}

// RowsAffected 不能用来判断记录是否存在：驱动没开 clientFoundRows，MySQL 报的是
// 「改变」的行数，而 updated_at 是秒精度——原样再存一次就是 0。所以先数一次。
func TestUpdateChecksExistenceBeforeWriting(t *testing.T) {
	f := newArticleFixture(t)
	f.rec.setRowCount(1)

	if err := f.repo.Update(context.Background(), "a7k2m9pq", ArticleUpdate{UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("更新失败：%v", err)
	}

	if count, update := f.rec.indexOf(t, "SELECT count(*)"), f.rec.indexOf(t, "UPDATE `articles`"); count > update {
		t.Errorf("应该先数一次再更新，实际顺序反了：\n%s", strings.Join(f.rec.all(), "\n"))
	}
}

func TestUpdateOnMissingRowWritesNothing(t *testing.T) {
	f := newArticleFixture(t)

	err := f.repo.Update(context.Background(), "a7k2m9pq", ArticleUpdate{Slug: "x", UpdatedAt: time.Now().UTC()})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("应报 ErrNotFound，实际 %v", err)
	}
	f.rec.reject(t, "UPDATE `articles`")
}

func TestDeleteTargetsOneRow(t *testing.T) {
	f := newArticleFixture(t)

	// DryRun 下驱动不会执行，RowsAffected 永远是 0，所以「删到空行就报
	// ErrNotFound」这一支在这里跑不到——那条路径靠 Delete 的实参 SQL 形状来钉。
	_ = f.repo.Delete(context.Background(), "a7k2m9pq")

	f.rec.expect(t, "DELETE FROM `articles`", "WHERE id = 'a7k2m9pq'")
}

func TestListAdminNeverSelectsBody(t *testing.T) {
	f := newArticleFixture(t)

	if _, _, err := f.repo.ListAdmin(context.Background(), AdminArticleFilter{}, 20, 0); err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	// 先确认列是显式列出来的——否则一旦退回 `SELECT *`，下面那句「不含 body」
	// 会因为整行取回来而不含字面量 body 而白白通过。
	statement := f.rec.expect(t, "`articles`.`title`")
	// 认列名本身，不认子串「body」：body_runes 名字里也带 body，而列表该带上它。
	if strings.Contains(statement, "`body`") {
		t.Errorf("后台列表不该查正文：\n%s", statement)
	}
	if !strings.Contains(statement, "`body_runes`") {
		t.Errorf("后台列表要带正文字数——它是一列数，不必读正文：\n%s", statement)
	}
}

func TestListAdminFilters(t *testing.T) {
	cases := []struct {
		name   string
		filter AdminArticleFilter
		want   []string
		absent []string
	}{
		{
			// 状态留空是「全都要」，不是「状态为空串」。
			name:   "不限状态时不加状态条件",
			filter: AdminArticleFilter{},
			want:   []string{"FROM `articles`"},
			absent: []string{"status =", "category =", "LIKE"},
		},
		{
			name:   "按状态",
			filter: AdminArticleFilter{Status: "draft"},
			want:   []string{"status = 'draft'"},
		},
		{
			name:   "按分类",
			filter: AdminArticleFilter{Category: aiCat},
			want:   []string{"category = '" + aiCat + "'"},
		},
		{
			name:   "关键词同时匹配标题和摘要",
			filter: AdminArticleFilter{Keyword: "go"},
			want:   []string{"title LIKE '%go%'", "summary LIKE '%go%'"},
		},
		{
			// 作者输的 % 不该当成通配符，否则「100%」会匹配到所有文章。
			name:   "关键词里的通配符被转义",
			filter: AdminArticleFilter{Keyword: "100%"},
			want:   []string{`LIKE '%100\%%'`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newArticleFixture(t)

			if _, _, err := f.repo.ListAdmin(context.Background(), tc.filter, 20, 0); err != nil {
				t.Fatalf("查询失败：%v", err)
			}

			for _, fragment := range tc.want {
				f.rec.expect(t, fragment)
			}
			for _, fragment := range tc.absent {
				f.rec.reject(t, fragment)
			}
		})
	}
}

func TestListAdminOrdersByUpdatedAt(t *testing.T) {
	f := newArticleFixture(t)

	if _, _, err := f.repo.ListAdmin(context.Background(), AdminArticleFilter{}, 20, 0); err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	// 后台按「最近改过的」排，公开列表才按首发时间。
	f.rec.expect(t, "ORDER BY updated_at desc,id asc")
}

// 总数和列表必须在同一个快照里读，否则翻页时会看到两次查询之间的增删。
func TestListAdminReadsInOneRepeatableReadTransaction(t *testing.T) {
	f := newArticleFixture(t)

	if _, _, err := f.repo.ListAdmin(context.Background(), AdminArticleFilter{}, 20, 0); err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	opts, ok := f.conn.lastTxOptions()
	if !ok {
		t.Fatal("没有开事务")
	}
	if !opts.ReadOnly {
		t.Error("只读事务不该能写")
	}
	if sql.IsolationLevel(opts.Isolation) != sql.LevelRepeatableRead {
		t.Errorf("应为可重复读，实际 %d", opts.Isolation)
	}
}

// 读者那一份带着分类名一起回来：名字在另一张表上，而读者那一页不该为了每篇的落款
// 再问一次 /categories——那一次要是失败了，文章就成了一排没有落款的条目。
func TestListPublishedBringsCategoryNames(t *testing.T) {
	f := newArticleFixture(t)
	f.rec.setArticles([]model.Article{
		{ID: "a7k2m9pq", Category: backendCat},
		{ID: "b8l3n0qr", Category: notesCat},
		{ID: "c9m4p1rs", Category: "gone"},
	})
	f.rec.setCategories([]model.Category{
		{ID: backendCat, Name: "后端开发"},
		{ID: notesCat, Name: "学习随笔"},
	})

	articles, _, err := f.repo.ListPublished(context.Background(), "", "", 20, 0)
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if len(articles) != 3 {
		t.Fatalf("应回 3 篇，实际 %d", len(articles))
	}

	if articles[0].CategoryName != "后端开发" || articles[1].CategoryName != "学习随笔" {
		t.Errorf("分类名没有对上：%q / %q", articles[0].CategoryName, articles[1].CategoryName)
	}
	// 外键拦着，指着不存在分类的文章不该存得下来；真撞上了也只是名字空着，
	// 页面照样出，不该整页读不出来。
	if articles[2].CategoryName != "" {
		t.Errorf("对不上的分类应留空，实际 %q", articles[2].CategoryName)
	}

	// 名字和文章读在同一个事务里：分两次读，一页里会出现一半新一半旧的名字。
	if begins := f.conn.beginCount(); begins != 1 {
		t.Errorf("文章和分类名应在同一个事务里读完，实际开了 %d 个", begins)
	}
}

func TestListPublishedSkipsTheLookupWhenThereIsNothingToName(t *testing.T) {
	f := newArticleFixture(t)
	f.rec.setCategories([]model.Category{{ID: backendCat, Name: "后端开发"}})

	if _, _, err := f.repo.ListPublished(context.Background(), "", "", 20, 0); err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	// 一页一篇文章都没有的时候还去读分类表，是白跑一趟。
	f.rec.reject(t, "FROM `categories`")
}

func TestGetPublishedByIDBringsTheCategoryName(t *testing.T) {
	f := newArticleFixture(t)
	// 详情那一条是 SELECT *，它只带得回文章自己的列，名字得再问一次分类表。
	f.rec.setArticles([]model.Article{{ID: "a7k2m9pq", Category: backendCat}})
	f.rec.setCategories([]model.Category{{ID: backendCat, Name: "后端开发"}})

	article, err := f.repo.GetPublishedByID(context.Background(), "a7k2m9pq")
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	if article.CategoryName != "后端开发" {
		t.Errorf("分类名应为「后端开发」，实际 %q", article.CategoryName)
	}
}

func TestGetPublishedByIDPinsStatus(t *testing.T) {
	f := newArticleFixture(t)

	if _, err := f.repo.GetPublishedByID(context.Background(), "a7k2m9pq"); err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("查询失败：%v", err)
	}

	// 公开接口按 id 查，但也必须钉住 published——少了这一条，草稿直接漏出去。
	f.rec.expect(t, "FROM `articles`", "id = 'a7k2m9pq' AND status = 'published'")
}

func TestGetByIDAllowsDrafts(t *testing.T) {
	f := newArticleFixture(t)

	if _, err := f.repo.GetByID(context.Background(), "a7k2m9pq"); err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("查询失败：%v", err)
	}

	statement := f.rec.expect(t, "FROM `articles`", "id = 'a7k2m9pq'")
	if strings.Contains(statement, "status") {
		t.Errorf("后台取单篇不该按状态过滤：\n%s", statement)
	}
}
