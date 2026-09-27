// 命令 mailsmoke 只做一件事：按 .env 里的配置真发一封信。
//
// 它绕开注册流程，所以不需要数据库就能验证发信通道——连不上、认证不过、
// 发信域名没验证，这些原因都会在这里立刻现形，而注册那边只会留下一行日志。
// 收件人默认是发信地址自己，也可以从命令行给一个。
package main

import (
	"context"
	"log"
	"os"
	"time"

	"anxin-hitsz.com/backend/internal/config"
	"anxin-hitsz.com/backend/internal/mail"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Mail == nil {
		log.Fatal("没有配置 SMTP_HOST，这台机器不发信")
	}

	to := cfg.Mail.From
	if len(os.Args) > 1 && os.Args[1] != "" {
		to = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	started := time.Now()
	err = mail.NewSMTPMailer(*cfg.Mail).Send(ctx, mail.Message{
		To:      to,
		Subject: "发信通道验证",
		Text: "看到这封信就说明 SMTP 是通的：连接、TLS、认证、发信域名都过了。\n\n" +
			"这封信来自本地的 mailsmoke 命令，不是注册流程发出的。\n",
	})
	if err != nil {
		log.Fatalf("发送失败（耗时 %v）：%v", time.Since(started).Round(time.Millisecond), err)
	}
	log.Printf("发送成功（耗时 %v），收件人 %s", time.Since(started).Round(time.Millisecond), to)
}
