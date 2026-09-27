// 包 mail 负责把一封信送出去。它不认识账号、令牌或任何业务概念——
// 正文由 service 拼好，这里只管运输。
//
// 真实的 SMTP 通道（阿里云邮件推送）是第 5 步。在那之前只有 LogMailer，
// 而它会把正文整段写进日志——正文里有验证链接，也就是有令牌。
package mail

import (
	"context"
	"errors"
	"log"
)

var ErrNotConfigured = errors.New("邮件通道未配置")

type Message struct {
	To      string
	Subject string
	Text    string
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

// 只给开发环境用。它把令牌打进日志，这正是它在本地能派上用场的原因，
// 也正是它不能出现在生产的原因。main.go 按 APP_ENV 选实现，不靠自觉。
type LogMailer struct{}

func (LogMailer) Send(_ context.Context, message Message) error {
	log.Printf("[mail:dev] 收件人=%s 主题=%s\n%s", message.To, message.Subject, message.Text)
	return nil
}

// 生产环境的占位。第 5 步把它换成 SMTP 实现。
// 现在这个存在的意义，是当 SMTP_HOST 还没配时让注册直接失败——
// 失败总好过安静地把令牌写进生产日志。
type UnconfiguredMailer struct{}

func (UnconfiguredMailer) Send(context.Context, Message) error {
	return ErrNotConfigured
}
