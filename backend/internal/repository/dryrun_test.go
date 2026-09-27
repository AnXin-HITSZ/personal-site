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
)

// 这个驱动不连任何数据库。GORM 的 DryRun 会跳过语句执行，但 Transaction
// 仍然要真的开一个事务，所以 Begin 必须能成功，且不能有任何 SQL 落到驱动上。
type dryRunConnector struct{ conn *dryRunConn }

func (c dryRunConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c dryRunConnector) Driver() driver.Driver                        { return dryRunDriver{} }

type dryRunDriver struct{}

func (dryRunDriver) Open(string) (driver.Conn, error) { return nil, errors.New("不应被调用") }

type dryRunConn struct {
	mu     sync.Mutex
	begins int
}

func (c *dryRunConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("DryRun 下不该有语句执行")
}
func (c *dryRunConn) Close() error { return nil }
func (c *dryRunConn) Begin() (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.begins++
	return dryRunTx{}, nil
}
func (c *dryRunConn) beginCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.begins
}

type dryRunTx struct{}

func (dryRunTx) Commit() error   { return nil }
func (dryRunTx) Rollback() error { return nil }

type recorder struct {
	mu         sync.Mutex
	statements []string
}

func (r *recorder) record(tx *gorm.DB) {
	stmt := tx.Statement
	if stmt == nil || stmt.SQL.Len() == 0 {
		return
	}
	text := tx.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)

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

	return fixture{repo: NewAccount(db), rec: rec, conn: conn}
}
