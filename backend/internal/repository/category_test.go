package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"anxin-hitsz.com/backend/internal/model"
)

var testCategoryNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// 分类 id 一律是建分类时现取的 8 位随机码，夹具也照那个形状写（article_test.go
// 里的分类夹具一并引用这里）。
const (
	backendCat  = "b7k2m9pq"
	frontendCat = "f8n3s5t7"
	aiCat       = "a9w2y4z6"
	notesCat    = "n6h4j2m8"
)

type categoryFixture struct {
	repo *Category
	rec  *recorder
	conn *dryRunConn
}

func newCategoryFixture(t *testing.T) categoryFixture {
	t.Helper()
	db, rec, conn := newDryRunDB(t)
	return categoryFixture{repo: NewCategory(db), rec: rec, conn: conn}
}

func TestCategoryKeyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "名字撞了", err: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'AI 探索' for key 'categories.uk_categories_name'"}, want: ErrCategoryNameTaken},
		{name: "主键撞了", err: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'abcd1234' for key 'categories.PRIMARY'"}, want: ErrIDTaken},
		{name: "下面还有文章", err: &mysql.MySQLError{Number: 1451, Message: "Cannot delete or update a parent row"}, want: ErrCategoryInUse},
		{name: "分类不存在", err: &mysql.MySQLError{Number: 1452, Message: "Cannot add or update a child row"}, want: ErrCategoryNotFound},
		// 1062 但撞的不是我们认得的那两个索引：原样返回，别猜。
		{name: "别的唯一键", err: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'x' for key 'some_other_key'"}, want: nil},
		{name: "别的错误", err: errors.New("connection refused"), want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categoryKeyError(tc.err)
			if tc.want == nil {
				if !errors.Is(got, tc.err) {
					t.Errorf("应原样返回，实际 %v", got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Errorf("应为 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// 挪分类不动文章的 updated_at：动了的话，写作列表按最近改动排，一挪就是好几篇一起
// 跳到最前面，看着像今天重写了它们。
func TestDeleteAndMoveLeavesArticleTimestampsAlone(t *testing.T) {
	f := newCategoryFixture(t)
	f.rec.setDeleteRowCount(1)

	if err := f.repo.DeleteAndMove(context.Background(), backendCat, notesCat, testCategoryNow); err != nil {
		t.Fatalf("删除失败：%v", err)
	}

	move := f.rec.expect(t, "UPDATE `articles`", "'"+notesCat+"'", "'"+backendCat+"'")
	if strings.Contains(move, "updated_at") {
		t.Errorf("挪分类不该动文章的 updated_at：\n%s", move)
	}

	// 两件事必须在一个事务里：分成两次请求，中间断一次就留下一批指着已删分类的文章。
	if begins := f.conn.beginCount(); begins != 1 {
		t.Errorf("挪走文章和删分类应在同一个事务里，实际开了 %d 个", begins)
	}
	f.rec.expect(t, "DELETE FROM `categories`")
}

func TestDeleteAndMoveReportsMissingCategory(t *testing.T) {
	f := newCategoryFixture(t)
	// 删分类那一步一行都没删到：它已经不在了。
	f.rec.setDeleteRowCount(0)

	if err := f.repo.DeleteAndMove(context.Background(), backendCat, notesCat, testCategoryNow); !errors.Is(err, ErrNotFound) {
		t.Errorf("应为 %v，实际 %v", ErrNotFound, err)
	}
}

// 顺序是整份提交的，所以提交上来的必须是现在这一整份。少一个就会有一个分类拿不到
// 位置，多一个说明它是在这一页打开之后才建的——两种都不猜，也不动任何一行。
func TestReorderRefusesAnIncompleteSet(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
	}{
		{name: "少了一个", ids: []string{backendCat, frontendCat, aiCat}},
		{name: "多了一个", ids: []string{backendCat, frontendCat, aiCat, notesCat, "abcd1234"}},
		{name: "换了一个", ids: []string{backendCat, frontendCat, aiCat, "abcd1234"}},
		{name: "一个都没有", ids: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCategoryFixture(t)
			f.rec.setPlucked([]string{backendCat, frontendCat, aiCat, notesCat})

			if err := f.repo.Reorder(context.Background(), tc.ids, testCategoryNow); !errors.Is(err, ErrOrderMismatch) {
				t.Fatalf("应为 %v，实际 %v", ErrOrderMismatch, err)
			}
			f.rec.reject(t, "UPDATE `categories`")
		})
	}
}

func TestReorderWritesEveryPosition(t *testing.T) {
	f := newCategoryFixture(t)
	f.rec.setPlucked([]string{backendCat, frontendCat, aiCat})

	if err := f.repo.Reorder(context.Background(), []string{aiCat, backendCat, frontendCat}, testCategoryNow); err != nil {
		t.Fatalf("排序失败：%v", err)
	}

	for id, position := range map[string]string{aiCat: "0", backendCat: "1", frontendCat: "2"} {
		statement := f.rec.expect(t, "UPDATE `categories`", "'"+id+"'")
		if !strings.Contains(statement, "`sort_order`="+position) {
			t.Errorf("%s 应写成位置 %s：\n%s", id, position, statement)
		}
	}

	if begins := f.conn.beginCount(); begins != 1 {
		t.Errorf("整张表应在同一个事务里写完，实际开了 %d 个", begins)
	}
}

// 两次并发的「加上」都先数到 19、再各插一行，就是 21 个。COUNT 上加 FOR UPDATE，
// 把已有的分类行锁住，这两次才会排成一队。
//
// 新分类拿到的位置这一条测不了：DryRun 下最后一名的查询一行都回不来，位置永远
// 落在「取不到行」那一支上。这里钉的是它在插入之前真的读了这一趟。
func TestCreateLocksTheCountBeforeInserting(t *testing.T) {
	f := newCategoryFixture(t)

	category := &model.Category{ID: "abcd1234", Name: "读书笔记", CreatedAt: testCategoryNow, UpdatedAt: testCategoryNow}
	if err := f.repo.Create(context.Background(), category, 20); err != nil {
		t.Fatalf("新建失败：%v", err)
	}

	f.rec.expect(t, "SELECT count(*) FROM `categories`", "FOR UPDATE")
	f.rec.expect(t, "SELECT `sort_order` FROM `categories`", "ORDER BY sort_order desc", "LIMIT 1")
	f.rec.expect(t, "INSERT INTO `categories`", "'读书笔记'")
}

func TestCreateRefusesPastTheLimit(t *testing.T) {
	f := newCategoryFixture(t)
	f.rec.setRowCount(20)

	err := f.repo.Create(context.Background(),
		&model.Category{ID: "abcd1234", Name: "读书笔记", CreatedAt: testCategoryNow, UpdatedAt: testCategoryNow}, 20)
	if !errors.Is(err, ErrTooManyCategories) {
		t.Fatalf("应为 %v，实际 %v", ErrTooManyCategories, err)
	}
	f.rec.reject(t, "INSERT INTO `categories`")
}

func TestListWithUsageReadsCountsAndOrderInOnePass(t *testing.T) {
	f := newCategoryFixture(t)

	if _, err := f.repo.ListWithUsage(context.Background()); err != nil {
		t.Fatalf("查询失败：%v", err)
	}

	f.rec.expect(t, "SELECT * FROM `categories`", "ORDER BY sort_order asc", "id asc")
	// 三个数都在这一趟里读：分两次读，中间那一瞬间「4 篇」和「其中 1 篇草稿」会对不上。
	f.rec.expect(t, "SELECT category, COUNT(*) AS n FROM `articles`", "GROUP BY `category`")
	f.rec.expect(t, "SELECT category, COUNT(*) AS n FROM `articles`", "status")
	// 读者那一份连「最近一篇」一起数：篇数和最近时间来自同一个分组、同一个快照。
	f.rec.expect(t, "MAX(published_at) AS latest", "status", "GROUP BY `category`")
}

func TestRenameTouchesOnlyTheName(t *testing.T) {
	f := newCategoryFixture(t)

	if err := f.repo.Rename(context.Background(), backendCat, "服务端", testCategoryNow); err != nil {
		t.Fatalf("改名失败：%v", err)
	}

	statement := f.rec.expect(t, "UPDATE `categories`", "'"+backendCat+"'")
	if !strings.Contains(statement, "`name`='服务端'") {
		t.Errorf("应把名字写进去：\n%s", statement)
	}
	// 顺序不在这一条里：改名字不改它在读者那一行里的位置。
	if strings.Contains(statement, "sort_order") {
		t.Errorf("改名不该动顺序：\n%s", statement)
	}
}
