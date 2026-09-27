package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"anxin-hitsz.com/backend/internal/mail"
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

// 一个都不填是合法的——那表示这台机器不发信。但只要填了主机，剩下的
// 就必须齐全：半套配置要是被当成「没配」，注册会照常返回 202，
// 而信永远发不出去，谁也不会发现。
func TestMailConfig(t *testing.T) {
	base := map[string]string{
		"MYSQL_HOST":     "127.0.0.1",
		"MYSQL_DATABASE": "personal_site_dev",
		"MYSQL_USER":     "personal_site_app",
		"MYSQL_PASSWORD": "fake-password-for-test-only",
	}
	const password = "fake-smtp-password-for-test-only"

	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
		want    *mail.SMTPConfig
	}{
		{name: "一个都不填表示不发信", env: nil},
		{name: "主机留空也当作没配", env: map[string]string{"SMTP_HOST": ""}},
		{
			name: "配全了照抄，端口默认 465，发件地址归一化",
			env: map[string]string{
				"SMTP_HOST":     "smtpdm.aliyun.com",
				"SMTP_USERNAME": "noreply@example.com",
				"SMTP_PASSWORD": password,
				"SMTP_FROM":     "Noreply@Example.com",
			},
			want: &mail.SMTPConfig{
				Host: "smtpdm.aliyun.com", Port: 465,
				Username: "noreply@example.com", Password: password,
				From: "noreply@example.com",
			},
		},
		{name: "只填主机是错误", env: map[string]string{"SMTP_HOST": "smtpdm.aliyun.com"}, wantErr: true},
		{
			name: "端口越界是错误",
			env: map[string]string{
				"SMTP_HOST": "smtpdm.aliyun.com", "SMTP_PORT": "70000",
				"SMTP_USERNAME": "noreply@example.com", "SMTP_PASSWORD": password,
				"SMTP_FROM": "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name: "发件地址带显示名是错误",
			env: map[string]string{
				"SMTP_HOST": "smtpdm.aliyun.com", "SMTP_USERNAME": "noreply@example.com",
				"SMTP_PASSWORD": password, "SMTP_FROM": "站点 <noreply@example.com>",
			},
			wantErr: true,
		},
		{
			name: "发件地址里夹换行是错误",
			env: map[string]string{
				"SMTP_HOST": "smtpdm.aliyun.com", "SMTP_USERNAME": "noreply@example.com",
				"SMTP_PASSWORD": password, "SMTP_FROM": "a@b.c\r\nBcc: victim@example.com",
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for key, value := range base {
				t.Setenv(key, value)
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("这份配置应当被拒")
				}
				if strings.Contains(err.Error(), password) {
					t.Fatal("报错信息里不能带出口令")
				}
				return
			}
			if err != nil {
				t.Fatalf("加载配置失败：%v", err)
			}
			if tc.want == nil {
				if cfg.Mail != nil {
					t.Fatalf("不该有邮件配置，实际 %v", cfg.Mail)
				}
				return
			}
			if cfg.Mail == nil {
				t.Fatal("应当有邮件配置")
			}
			if *cfg.Mail != *tc.want {
				t.Errorf("邮件配置应为 %+v，实际 %+v", *tc.want, *cfg.Mail)
			}
		})
	}
}

// 这个结构迟早会被谁顺手打进日志。
func TestSMTPConfigStringHidesPassword(t *testing.T) {
	const password = "fake-smtp-password-for-test-only"
	cfg := mail.SMTPConfig{Host: "smtpdm.aliyun.com", Port: 465,
		Username: "noreply@example.com", Password: password, From: "noreply@example.com"}
	if strings.Contains(cfg.String(), password) {
		t.Fatal("config string disclosed password")
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
