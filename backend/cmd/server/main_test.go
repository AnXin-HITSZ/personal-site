package main

import (
	"reflect"
	"testing"

	"anxin-hitsz.com/backend/internal/config"
	"anxin-hitsz.com/backend/internal/mail"
)

// 生产环境没配 SMTP 时绝不能落到 LogMailer 上：它会把验证链接整段打进日志，
// 而日志比邮件好捞得多。这条分支没有别的看门人——配置文件错了不会报错，
// 只会安静地换一个实现，所以在这里钉死。
func TestNewMailerSelection(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want mail.Mailer
	}{
		{
			name: "配了 SMTP 就用真的，开发机上也是",
			cfg:  config.Config{AppEnv: "development", Mail: &mail.SMTPConfig{Host: "smtpdm.aliyun.com"}},
			want: &mail.SMTPMailer{},
		},
		{
			name: "生产没配退到会失败的占位",
			cfg:  config.Config{AppEnv: "production"},
			want: mail.UnconfiguredMailer{},
		},
		{
			name: "开发没配才用日志",
			cfg:  config.Config{AppEnv: "development"},
			want: mail.LogMailer{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newMailer(tc.cfg)
			if reflect.TypeOf(got) != reflect.TypeOf(tc.want) {
				t.Fatalf("应当选 %T，实际 %T", tc.want, got)
			}
		})
	}
}
