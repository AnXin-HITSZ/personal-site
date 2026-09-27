// 包 mail 负责把一封信送出去。它不认识账号、令牌或任何业务概念——
// 正文由 service 拼好，这里只管运输。
//
// 三个实现，由 main.go 按配置和环境挑：SMTPMailer 真的发信；LogMailer 把
// 正文打进日志，只给开发环境用（正文里有验证链接，也就是有令牌）；
// UnconfiguredMailer 一律失败，给没配 SMTP 的生产环境用——失败好过安静地
// 把令牌写进生产日志。
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

// 生产环境没配 SMTP_HOST 时的兜底。接口照旧返回「验证邮件已经发出」，
// 但信不会发出——启动日志里有一行警告，那是唯一能提前发现它的地方。
type UnconfiguredMailer struct{}

func (UnconfiguredMailer) Send(context.Context, Message) error {
	return ErrNotConfigured
}
