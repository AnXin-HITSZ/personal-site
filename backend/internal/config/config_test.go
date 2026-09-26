package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestLoadDoesNotLeakMalformedEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	const secret = "fake-secret-for-test-only"
	if err := os.WriteFile(filepath.Join(".", ".env"), []byte("MYSQL_PASSWORD=\""+secret), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("malformed .env must fail without disclosing its contents")
	}
}

func TestDSNRoundTrip(t *testing.T) {
	cfg := MySQLConfig{Host: "127.0.0.1", Port: 13306, Database: "personal_site_dev",
		User: "personal_site_app", Password: "fake:@/?&secret", Charset: "utf8mb4", Location: time.UTC}
	parsed, err := mysql.ParseDSN(cfg.DSN())
	if err != nil {
		t.Fatal("DSN could not be parsed")
	}
	if parsed.Passwd != cfg.Password || parsed.Addr != "127.0.0.1:13306" || parsed.DBName != cfg.Database || !parsed.ParseTime || parsed.Loc != time.UTC {
		t.Fatal("DSN did not preserve connection settings")
	}
	if parsed.Timeout <= 0 || parsed.ReadTimeout <= 0 || parsed.WriteTimeout <= 0 {
		t.Fatal("driver network timeouts must be bounded")
	}
	if strings.Contains(cfg.String(), cfg.Password) {
		t.Fatal("config string disclosed password")
	}
}
