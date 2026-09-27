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

// APP_ENV 决定 cookie 带不带 Secure：生产只走 HTTPS，开发是 http://localhost:5173，
// 带错了不是报错，是 cookie 静默地存不下或发不出去。
func TestSessionConfig(t *testing.T) {
	base := map[string]string{
		"MYSQL_HOST":     "127.0.0.1",
		"MYSQL_DATABASE": "personal_site_dev",
		"MYSQL_USER":     "personal_site_app",
		"MYSQL_PASSWORD": "fake-password-for-test-only",
	}

	cases := []struct {
		name        string
		appEnv      string
		origins     string
		wantSecure  bool
		wantOrigins []string
	}{
		{name: "开发环境不要求 Secure", appEnv: "development", wantSecure: false},
		{name: "生产环境要求 Secure", appEnv: "production", wantSecure: true},
		{name: "没配 APP_ENV 时按开发处理", appEnv: "", wantSecure: false},
		{
			name:        "来源按逗号拆开，去掉空白和空项",
			appEnv:      "production",
			origins:     " https://a.example , ,http://localhost:5173 ,",
			wantSecure:  true,
			wantOrigins: []string{"https://a.example", "http://localhost:5173"},
		},
		{name: "全空白等于没配", appEnv: "production", origins: " , ", wantSecure: true, wantOrigins: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for key, value := range base {
				t.Setenv(key, value)
			}
			t.Setenv("APP_ENV", tc.appEnv)
			t.Setenv("ALLOWED_ORIGINS", tc.origins)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("加载配置失败：%v", err)
			}
			if cfg.Session.CookieSecure != tc.wantSecure {
				t.Errorf("CookieSecure 应为 %v，实际 %v", tc.wantSecure, cfg.Session.CookieSecure)
			}
			if strings.Join(cfg.Session.AllowedOrigins, ",") != strings.Join(tc.wantOrigins, ",") {
				t.Errorf("AllowedOrigins 应为 %v，实际 %v", tc.wantOrigins, cfg.Session.AllowedOrigins)
			}
		})
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
