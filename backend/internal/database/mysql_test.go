package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/config"
)

// This connector does not contact any real database or read credentials.
type testConnector struct{ conn *testConn }

func (c testConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c testConnector) Driver() driver.Driver                        { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type testConn struct {
	closed  bool
	pingErr error
}

func (c *testConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected SQL") }
func (c *testConn) Close() error                        { c.closed = true; return nil }
func (c *testConn) Begin() (driver.Tx, error)           { return nil, errors.New("unexpected transaction") }
func (c *testConn) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	return c.pingErr
}

func TestInitializeAndClose(t *testing.T) {
	conn := &testConn{}
	pool := sql.OpenDB(testConnector{conn})
	cfg := config.MySQLConfig{MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: time.Minute}
	db, err := initialize(context.Background(), cfg, pool)
	if err != nil {
		t.Fatal(err)
	}
	if db.DB == nil || pool.Stats().MaxOpenConnections != 10 {
		t.Fatal("missing GORM handle or pool settings")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if !conn.closed {
		t.Fatal("connection was not closed")
	}
}

func TestPingFailureClosesPoolAndRedactsError(t *testing.T) {
	conn := &testConn{pingErr: errors.New("fake-sensitive-driver-message")}
	pool := sql.OpenDB(testConnector{conn})
	_, err := initialize(context.Background(), config.MySQLConfig{MaxOpenConns: 1, MaxIdleConns: 1}, pool)
	if err == nil || strings.Contains(err.Error(), "fake-sensitive") {
		t.Fatal("expected sanitized failure")
	}
	if !conn.closed {
		t.Fatal("failed initialization leaked connection")
	}
	if pool.Ping() == nil {
		t.Fatal("failed initialization left pool open")
	}
}

func TestCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := sql.OpenDB(testConnector{&testConn{}})
	_, err := initialize(ctx, config.MySQLConfig{}, pool)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if pool.Ping() == nil {
		t.Fatal("cancelled initialization left pool open")
	}
}
