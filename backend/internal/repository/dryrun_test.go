package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"anxin-hitsz.com/backend/internal/model"
)

// 这个驱动不连任何数据库。GORM 的 DryRun 会跳过语句执行，但 Transaction
// 仍然要真的开一个事务，所以 Begin 必须能成功，且不能有任何 SQL 落到驱动上。
type dryRunConnector struct{ conn *dryRunConn }

func (c dryRunConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c dryRunConnector) Driver() driver.Driver                        { return dryRunDriver{} }

type dryRunDriver struct{}

func (dryRunDriver) Open(string) (driver.Conn, error) { return nil, errors.New("不应被调用") }

type dryRunConn struct {
	mu        sync.Mutex
	begins    int
	txOptions []driver.TxOptions
}

func (c *dryRunConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("DryRun 下不该有语句执行")
}
func (c *dryRunConn) Close() error { return nil }

func (c *dryRunConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// database/sql 只在驱动实现了 ConnBeginTx 时才会把隔离级别和只读透传下去，
// 否则直接报「driver does not support non-default isolation level」。
// 这里顺便把选项记下来，事务的隔离级别才有的可测。
func (c *dryRunConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.begins++
	c.txOptions = append(c.txOptions, opts)
	return dryRunTx{}, nil
}

func (c *dryRunConn) beginCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.begins
}

func (c *dryRunConn) lastTxOptions() (driver.TxOptions, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.txOptions) == 0 {
		return driver.TxOptions{}, false
	}
	return c.txOptions[len(c.txOptions)-1], true
}

type dryRunTx struct{}

func (dryRunTx) Commit() error   { return nil }
func (dryRunTx) Rollback() error { return nil }

type recorder struct {
	mu         sync.Mutex
	statements []string

	// DryRun 下查询根本不执行，Count 拿到的永远是 0，而被测的 Update 正是用
	// Count 判存在——不把这个数补回去，每条更新都会在测试里被当成「记录不存在」。
	// 默认 0，那恰好就是记录真的不存在时 MySQL 会给的答案。
	rowCount int64

	// 同一个道理，Pluck 也拿不到行。整份提交的顺序要和库里现有的那一份对一遍，
	// 不给回去的话，Reorder 的每一条都会走进「对不上」那一支。默认 nil 表示
	// 不插手，于是 *[]string 留空——那也是真查不到东西时它的样子。
	plucked []string

	// Delete 的 RowsAffected 同样来自驱动，DryRun 下永远是 0。默认 0 就是
	// 「一行都没删到」，正好是记录真的不存在时它该有的样子。
	deleted int64

	// 文章和分类也查不出行来。读者那一份要带着分类名回来，而名字是拿到文章之后
	// 才去对上的——两边都不给回几行，那一段就没得测。默认 nil 表示不插手。
	articles   []model.Article
	categories []model.Category
}

// 只影响 count 查询：Dest 是 *int64 且语句里有 count(*)。
func (r *recorder) answerRowCount(tx *gorm.DB) {
	r.mu.Lock()
	count := r.rowCount
	r.mu.Unlock()

	dest, ok := tx.Statement.Dest.(*int64)
	if !ok || !strings.Contains(tx.Statement.SQL.String(), "count(*)") {
		return
	}
	*dest = count
	// Count 看到 RowsAffected != 1 时会拿它反过来覆盖 *count，所以这里一起摆平。
	tx.RowsAffected = 1
}

// 只影响 Pluck 到 *[]string 的那种查询。
func (r *recorder) answerPluck(tx *gorm.DB) {
	r.mu.Lock()
	ids := r.plucked
	r.mu.Unlock()

	if ids == nil {
		return
	}
	dest, ok := tx.Statement.Dest.(*[]string)
	if !ok {
		return
	}
	*dest = append([]string(nil), ids...)
	tx.RowsAffected = int64(len(ids))
}

// 只影响 DELETE 语句。
func (r *recorder) answerDelete(tx *gorm.DB) {
	r.mu.Lock()
	deleted := r.deleted
	r.mu.Unlock()

	if deleted == 0 {
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(tx.Statement.SQL.String()), "DELETE") {
		return
	}
	tx.RowsAffected = deleted
}

// 文章和分类的查询：Dest 是它们的切片，或者单个模型（详情那一条）。
func (r *recorder) answerRows(tx *gorm.DB) {
	r.mu.Lock()
	articles, categories := r.articles, r.categories
	r.mu.Unlock()

	switch dest := tx.Statement.Dest.(type) {
	case *[]model.Article:
		if articles == nil {
			return
		}
		*dest = append([]model.Article(nil), articles...)
		tx.RowsAffected = int64(len(articles))
	case *model.Article:
		if len(articles) == 0 {
			return
		}
		*dest = articles[0]
		tx.RowsAffected = 1
	case *[]model.Category:
		if categories == nil {
			return
		}
		*dest = append([]model.Category(nil), categories...)
		tx.RowsAffected = int64(len(categories))
	}
}

func (r *recorder) setRowCount(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rowCount = n
}

func (r *recorder) setDeleteRowCount(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = n
}

// nil 表示不插手，让查询自己给出「一行都没有」。
func (r *recorder) setPlucked(ids []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plucked = ids
}

func (r *recorder) setArticles(articles []model.Article) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.articles = articles
}

func (r *recorder) setCategories(categories []model.Category) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.categories = categories
}

func (r *recorder) record(tx *gorm.DB) {
	stmt := tx.Statement
	if stmt == nil || stmt.SQL.Len() == 0 {
		return
	}
	text := tx.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
	r.answerRowCount(tx)
	r.answerPluck(tx)
	r.answerDelete(tx)
	r.answerRows(tx)

	r.mu.Lock()
	r.statements = append(r.statements, text)
	r.mu.Unlock()
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statements...)
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = nil
}

// expect 断言某条语句同时包含全部片段，并返回它。
func (r *recorder) expect(t *testing.T, fragments ...string) string {
	t.Helper()
	for _, statement := range r.all() {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(statement, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return statement
		}
	}
	t.Fatalf("没有语句同时包含 %q，实际生成：\n%s", fragments, strings.Join(r.all(), "\n"))
	return ""
}

// reject 断言没有任何语句包含给定片段。
func (r *recorder) reject(t *testing.T, fragment string) {
	t.Helper()
	for _, statement := range r.all() {
		if strings.Contains(statement, fragment) {
			t.Fatalf("语句不该包含 %q：\n%s", fragment, statement)
		}
	}
}

func (r *recorder) indexOf(t *testing.T, fragment string) int {
	t.Helper()
	for i, statement := range r.all() {
		if strings.Contains(statement, fragment) {
			return i
		}
	}
	t.Fatalf("没有语句包含 %q，实际生成：\n%s", fragment, strings.Join(r.all(), "\n"))
	return -1
}

type fixture struct {
	repo *Account
	rec  *recorder
	conn *dryRunConn
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	db, rec, conn := newDryRunDB(t)
	return fixture{repo: NewAccount(db), rec: rec, conn: conn}
}

// 不连任何数据库的 GORM 实例，外加一个把每条语句记下来的回调。
func newDryRunDB(t *testing.T) (*gorm.DB, *recorder, *dryRunConn) {
	t.Helper()

	conn := &dryRunConn{}
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(dryRunConnector{conn: conn}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("构造 DryRun 连接失败：%v", err)
	}

	rec := &recorder{}
	hooks := []struct {
		register func(string, func(*gorm.DB)) error
		name     string
	}{
		{db.Callback().Create().After("gorm:create").Register, "test:record_create"},
		{db.Callback().Query().After("gorm:query").Register, "test:record_query"},
		{db.Callback().Update().After("gorm:update").Register, "test:record_update"},
		{db.Callback().Delete().After("gorm:delete").Register, "test:record_delete"},
		{db.Callback().Row().After("gorm:row").Register, "test:record_row"},
	}
	for _, hook := range hooks {
		if err := hook.register(hook.name, rec.record); err != nil {
			t.Fatalf("注册回调失败：%v", err)
		}
	}

	return db, rec, conn
}
