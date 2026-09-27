package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// 一次投递从连上到 QUIT 的总预算。net/smtp 全程不认识 context，真正管住
// 时间的是挂在连接上的 deadline；这个值同时也是账号接口最慢的响应时间——
// service 是同步发信的，用户要在注册那一屏等它结束。
const sendTimeout = 10 * time.Second

// 465 是隐式 TLS：连上就是密的。其余端口先明文，再要求 STARTTLS 升级。
const implicitTLSPort = 465

// SMTP 给一行的上限是 1000 字节（RFC 5321），base64 的规范换行是 76 字符
// （RFC 2045）。取 72 是留余量，反正多几行不花钱。
const base64LineWidth = 72

var (
	ErrRecipientMissing = errors.New("收件人地址为空")
	ErrHeaderInjection  = errors.New("邮件头不能包含换行")
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// 和 MySQLConfig 同样的理由：这个结构迟早会被谁顺手打进日志，
// 而口令不该以任何形式出现在日志里。
func (c SMTPConfig) String() string {
	return fmt.Sprintf("SMTPConfig{Host:%s Port:%d Username:%s Password:*** From:%s}",
		c.Host, c.Port, c.Username, c.From)
}

type SMTPMailer struct {
	config  SMTPConfig
	timeout time.Duration
	now     func() time.Time
	// 拨号抽成字段，测试才能指向本地起的一个假服务器：真实那条路在 465 上
	// 走隐式 TLS，而测试里造不出一张被系统信任的证书。
	dial func(address string, deadline time.Time) (net.Conn, error)
}

func NewSMTPMailer(config SMTPConfig) *SMTPMailer {
	mailer := &SMTPMailer{config: config, timeout: sendTimeout, now: time.Now}
	mailer.dial = mailer.connect
	return mailer
}

// 不用 tls.DialWithDialer：它的握手跑在 context.Background 上，握到一半
// 不吭声的服务器能一直挂在这里——而期限本来是要管住整次投递的。
func (m *SMTPMailer) connect(address string, deadline time.Time) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: m.timeout}
	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	// 先挂期限再握手，超时才算数。
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}
	if m.config.Port != implicitTLSPort {
		return conn, nil
	}

	secure := tls.Client(conn, m.tlsConfig())
	if err := secure.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	return secure, nil
}

func (m *SMTPMailer) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: m.config.Host, MinVersion: tls.VersionTLS12}
}

func (m *SMTPMailer) Send(ctx context.Context, message Message) error {
	if err := checkMessage(m.config.From, message); err != nil {
		return err
	}
	// 请求已经被取消就不必再连了。开始之后就不再检查 context：net/smtp
	// 没法中途打断，能兑现的只有连接上的那个期限。
	if err := ctx.Err(); err != nil {
		return err
	}

	messageID, err := newMessageID(m.config.From)
	if err != nil {
		return err
	}

	// 一次投递只有这一个期限：TCP 连接、TLS 握手、整段 SMTP 对话都算在里面。
	// 分开算的话最坏情况会翻倍，而 nginx 那边的 proxy_read_timeout 是 15 秒。
	//
	// 这里用的是真实时间，不是 m.now：那个字段是给 Date 头用的，测试会把它
	// 钉死成某个时刻，而假时钟驱动不了真实的套接字期限。
	deadline := time.Now().Add(m.timeout)

	address := net.JoinHostPort(m.config.Host, strconv.Itoa(m.config.Port))
	conn, err := m.dial(address, deadline)
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败：%w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		return err
	}

	if err := m.upgrade(client); err != nil {
		return err
	}
	if err := client.Auth(smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)); err != nil {
		return fmt.Errorf("SMTP 认证失败：%w", err)
	}
	if err := client.Mail(m.config.From); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(m.render(message, messageID)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}

// 465 之外的端口一律要求升级到 TLS 之后才谈认证。PlainAuth 自己也拒绝在
// 明文连接上发出凭据，但那道防线是 net/smtp 的默认值；写在这里的理由是
// 让「凭据绝不走明文」这件事在代码里读得出来，而不是靠库的善意。
func (m *SMTPMailer) upgrade(client *smtp.Client) error {
	if m.config.Port == implicitTLSPort {
		return nil
	}
	return client.StartTLS(m.tlsConfig())
}

func (m *SMTPMailer) render(message Message, messageID string) []byte {
	var buf bytes.Buffer

	// 头必须是 ASCII：中文主题要按 RFC 2047 编成 =?UTF-8?B?...?=，
	// 否则收件方看到的是乱码，或者干脆被拒。
	writeHeader(&buf, "From", m.config.From)
	writeHeader(&buf, "To", message.To)
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("UTF-8", message.Subject))
	writeHeader(&buf, "Date", m.now().UTC().Format(time.RFC1123Z))
	writeHeader(&buf, "Message-ID", messageID)
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(wrapBase64(message.Text))

	return buf.Bytes()
}

// net/smtp 会挡住命令里的换行（Mail 和 Rcpt 都过一遍 validateLine），
// 但 DATA 段是原样写进去的：邮件头里的 CRLF 能让收件人凭空多出 Bcc、
// Content-Type 之类的头。收件人来自用户输入，这一层不能假设调用方过滤过。
func checkMessage(from string, message Message) error {
	if strings.ContainsAny(message.To, "\r\n") || strings.ContainsAny(message.Subject, "\r\n") ||
		strings.ContainsAny(from, "\r\n") {
		return ErrHeaderInjection
	}
	if message.To == "" {
		return ErrRecipientMissing
	}
	return nil
}

func writeHeader(buf *bytes.Buffer, name, value string) {
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

// 折行要自己来：base64.Encoder 只负责编码，不管行长。
func wrapBase64(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	var buf strings.Builder
	for len(encoded) > base64LineWidth {
		buf.WriteString(encoded[:base64LineWidth])
		buf.WriteString("\r\n")
		encoded = encoded[base64LineWidth:]
	}
	buf.WriteString(encoded)
	buf.WriteString("\r\n")
	return buf.String()
}

// 缺 Message-ID 会被一部分垃圾邮件过滤器扣分。域名取发件人那一半，
// 随机部分只为了两封信不撞。
func newMessageID(from string) (string, error) {
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		domain = from[at+1:]
	}

	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "<" + hex.EncodeToString(raw) + "@" + domain + ">", nil
}
