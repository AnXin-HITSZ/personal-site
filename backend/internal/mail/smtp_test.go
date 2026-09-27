package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

const testPassword = "fake-password-for-test-only"

// fakeServer 是一个只够跑完一次投递的 SMTP 服务器：按行应答，把 DATA 段
// 原样收下来，让用例检查真正发出去的字节。
//
// 它不会真的升级 TLS——所以 465 那条隐式加密的路由由 connect 里的 tls.Client
// 负责，这里覆盖不到。这个欠缺是有意的：造一张被系统信任的证书需要动进程的
// 根证书池，代价比它验证的那两行大。没覆盖到的是「加密本身」，覆盖到的是
// 「什么时候才允许发凭据」，而后者才是会写错的地方。
type fakeServer struct {
	listener net.Listener
	options  fakeOptions

	mu          sync.Mutex
	commands    []string
	messages    []string
	connections int
	done        chan struct{}
}

type fakeOptions struct {
	// 在 EHLO 里广告 STARTTLS。假服务器不会真的升级，这个开关只用来
	// 区分「服务器支持」和「服务器不支持」两种应答。
	startTLS bool
	// 在这一步回 550：写 "MAIL" 或 "RCPT"。
	reject string
	// 打完招呼就不再应答，也不关连接——用来验证期限真的在生效。
	stall bool
}

func newFakeServer(t *testing.T, options fakeOptions) *fakeServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了假服务器：%v", err)
	}
	server := &fakeServer{listener: listener, options: options, done: make(chan struct{})}

	go server.serve()
	t.Cleanup(func() {
		close(server.done)
		listener.Close()
	})
	return server
}

func (s *fakeServer) addr() string { return s.listener.Addr().String() }

func (s *fakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()

	s.mu.Lock()
	s.connections++
	s.mu.Unlock()

	reader := bufio.NewReader(conn)
	reply := func(line string) bool {
		_, err := fmt.Fprintf(conn, "%s\r\n", line)
		return err == nil
	}

	if !reply("220 fake ESMTP ready") {
		return
	}
	if s.options.stall {
		<-s.done
		return
	}

	var body strings.Builder
	inData := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.messages = append(s.messages, body.String())
				s.mu.Unlock()
				body.Reset()
				if !reply("250 queued") {
					return
				}
				continue
			}
			body.WriteString(line)
			body.WriteString("\n")
			continue
		}

		s.mu.Lock()
		s.commands = append(s.commands, line)
		s.mu.Unlock()

		switch {
		case hasVerb(line, "EHLO"), hasVerb(line, "HELO"):
			if !reply("250-fake greets you") {
				return
			}
			if s.options.startTLS && !reply("250-STARTTLS") {
				return
			}
			if !reply("250-8BITMIME") || !reply("250 SIZE 35882577") {
				return
			}
		case hasVerb(line, "AUTH"):
			if !reply("235 authentication succeeded") {
				return
			}
		case hasVerb(line, "MAIL"):
			if s.options.reject == "MAIL" {
				reply("550 sender rejected")
				continue
			}
			if !reply("250 ok") {
				return
			}
		case hasVerb(line, "RCPT"):
			if s.options.reject == "RCPT" {
				reply("550 mailbox unavailable")
				continue
			}
			if !reply("250 ok") {
				return
			}
		case hasVerb(line, "DATA"):
			if !reply("354 end with a line containing only a period") {
				return
			}
			inData = true
		case hasVerb(line, "QUIT"):
			reply("221 bye")
			return
		default:
			if !reply("500 command not recognised") {
				return
			}
		}
	}
}

func hasVerb(line, verb string) bool {
	return line == verb || strings.HasPrefix(line, verb+" ")
}

func (s *fakeServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

func (s *fakeServer) commandHistory() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func (s *fakeServer) lastMessage(t *testing.T) string {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		t.Fatal("假服务器没有收到任何报文")
	}
	return s.messages[len(s.messages)-1]
}

// Host 必须是 127.0.0.1：net/smtp 的 PlainAuth 拒绝在明文连接上发凭据，
// 只有对 localhost 才网开一面，而这里注入的拨号并不产生 TLS 连接。
func newTestMailer(t *testing.T, server *fakeServer, config SMTPConfig) *SMTPMailer {
	t.Helper()

	config.Host = "127.0.0.1"
	config.Port = implicitTLSPort
	if config.Username == "" {
		config.Username = "noreply@example.com"
	}
	if config.Password == "" {
		config.Password = testPassword
	}
	if config.From == "" {
		config.From = "noreply@example.com"
	}

	mailer := NewSMTPMailer(config)
	mailer.now = func() time.Time { return testNow }
	mailer.dial = func(string, time.Time) (net.Conn, error) {
		return net.Dial("tcp", server.addr())
	}
	return mailer
}

func testMessage() Message {
	return Message{
		To:      "reader@example.com",
		Subject: "验证你的邮箱",
		Text:    "点开下面的链接完成邮箱验证：\n\nhttps://example.com/verify-email?token=abc123\n",
	}
}

// 报文按头、正文拆开。假服务器把每行结尾统一成 \n，正文因此是一整块
// 折了行的 base64，去掉换行才能解回来。
func parseMessage(t *testing.T, raw string) (map[string]string, string) {
	t.Helper()

	head, body, found := strings.Cut(raw, "\n\n")
	if !found {
		t.Fatalf("报文里没有分隔头和正文的空行：%q", raw)
	}

	headers := map[string]string{}
	for _, line := range strings.Split(head, "\n") {
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("头行读不懂：%q", line)
		}
		headers[strings.ToLower(name)] = value
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\n", ""))
	if err != nil {
		t.Fatalf("正文不是合法的 base64：%v", err)
	}
	return headers, string(decoded)
}

func TestSMTPDeliversTheRenderedMessage(t *testing.T) {
	server := newFakeServer(t, fakeOptions{})
	mailer := newTestMailer(t, server, SMTPConfig{})

	if err := mailer.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("发送失败：%v", err)
	}

	headers, body := parseMessage(t, server.lastMessage(t))

	if headers["from"] != "noreply@example.com" {
		t.Errorf("From 应为 noreply@example.com，实际 %q", headers["from"])
	}
	if headers["to"] != "reader@example.com" {
		t.Errorf("To 应为 reader@example.com，实际 %q", headers["to"])
	}
	if headers["date"] != testNow.UTC().Format(time.RFC1123Z) {
		t.Errorf("Date 应为 RFC 5322 格式，实际 %q", headers["date"])
	}
	if headers["content-type"] != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type 不对：%q", headers["content-type"])
	}
	if headers["content-transfer-encoding"] != "base64" {
		t.Errorf("正文应当声明 base64 编码，实际 %q", headers["content-transfer-encoding"])
	}
	if !regexp.MustCompile(`^<[0-9a-f]{24}@example\.com>$`).MatchString(headers["message-id"]) {
		t.Errorf("Message-ID 形状不对：%q", headers["message-id"])
	}
	if !strings.Contains(body, "https://example.com/verify-email?token=abc123") {
		t.Errorf("正文里没有那个链接：%q", body)
	}

	// 命令顺序不是随便的：认证必须早于 MAIL，MAIL 必须早于 RCPT。
	want := []string{"EHLO", "AUTH", "MAIL", "RCPT", "DATA"}
	history := server.commandHistory()
	if len(history) < len(want)+1 {
		t.Fatalf("命令太少：%v", history)
	}
	for index, verb := range want {
		if !hasVerb(history[index], verb) {
			t.Errorf("第 %d 条命令应为 %s，实际 %q", index+1, verb, history[index])
		}
	}
	if !hasVerb(history[len(want)], "QUIT") {
		t.Errorf("最后一条命令应为 QUIT，实际 %q", history[len(want)])
	}
}

// 头必须是 ASCII。中文主题原样写进去，收件方看到的是乱码，严格一点的
// 中继直接退回。
func TestNonASCIISubjectIsEncodedNotRaw(t *testing.T) {
	server := newFakeServer(t, fakeOptions{})
	mailer := newTestMailer(t, server, SMTPConfig{})

	if err := mailer.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("发送失败：%v", err)
	}

	raw := server.lastMessage(t)
	if strings.Contains(raw, "验证你的邮箱") {
		t.Error("中文主题必须编码后再进头部，不能在报文里裸着出现")
	}

	headers, _ := parseMessage(t, raw)
	decoded, err := new(mime.WordDecoder).DecodeHeader(headers["subject"])
	if err != nil {
		t.Fatalf("主题不是合法的 RFC 2047 编码字：%v", err)
	}
	if decoded != "验证你的邮箱" {
		t.Errorf("主题应为「验证你的邮箱」，实际 %q", decoded)
	}
}

// DATA 段是原样写进去的：头里的 CRLF 能让收件人凭空多出一个 Bcc。
// 收件人来自用户输入，这一层不能假设调用方过滤过。
func TestHeaderInjectionIsRefusedBeforeDialling(t *testing.T) {
	cases := []struct {
		name    string
		message Message
		want    error
	}{
		{
			name:    "收件人里塞了换行",
			message: Message{To: "reader@example.com\r\nBcc: leak@example.com", Subject: "你好", Text: "正文"},
			want:    ErrHeaderInjection,
		},
		{
			name:    "主题里塞了换行",
			message: Message{To: "reader@example.com", Subject: "你好\r\nBcc: leak@example.com", Text: "正文"},
			want:    ErrHeaderInjection,
		},
		{name: "收件人为空", message: Message{Subject: "你好", Text: "正文"}, want: ErrRecipientMissing},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeServer(t, fakeOptions{})
			mailer := newTestMailer(t, server, SMTPConfig{})

			// 只断言「出了错」是不够的：拨号失败也一样出错，那样这条用例
			// 会在代码已经坏掉的时候继续变绿。
			if err := mailer.Send(context.Background(), tc.message); !errors.Is(err, tc.want) {
				t.Fatalf("应当返回 %v，实际 %v", tc.want, err)
			}
			if server.connectionCount() != 0 {
				t.Error("被拒的报文不该连出去")
			}
		})
	}
}

// 服务器不提供 STARTTLS 时必须失败，而不是降级成明文——降级的下一步
// 就是把口令明文递出去。
func TestCredentialsAreNotSentWhenSTARTTLSIsUnavailable(t *testing.T) {
	server := newFakeServer(t, fakeOptions{startTLS: false})
	mailer := newTestMailer(t, server, SMTPConfig{})

	// 465 走隐式加密，不会经过 STARTTLS 分支，所以要换一个端口。
	mailer.config.Port = 587

	err := mailer.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("服务器不支持 STARTTLS 时应当失败")
	}

	for _, command := range server.commandHistory() {
		if hasVerb(command, "AUTH") {
			t.Fatalf("凭据不能在明文连接上发出，实际发了 %q", command)
		}
	}
}

func TestServerRejectionIsReported(t *testing.T) {
	server := newFakeServer(t, fakeOptions{reject: "RCPT"})
	mailer := newTestMailer(t, server, SMTPConfig{})

	err := mailer.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("服务器退回收件人时应当失败")
	}
	if !strings.Contains(err.Error(), "550") {
		t.Errorf("错误里应当带上服务器的应答，实际 %v", err)
	}
}

// net/smtp 不认识 context，真正管住时间的是挂在连接上的期限。
// 不吭声的服务器不能把注册请求一起挂住。
func TestStalledServerHitsTheDeadline(t *testing.T) {
	server := newFakeServer(t, fakeOptions{stall: true})
	mailer := newTestMailer(t, server, SMTPConfig{})
	mailer.timeout = 100 * time.Millisecond

	started := time.Now()
	err := mailer.Send(context.Background(), testMessage())
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("服务器不应答时应当超时失败")
	}
	if elapsed > 5*time.Second {
		t.Errorf("期限应当在毫秒级生效，实际等了 %v", elapsed)
	}
}

func TestCancelledContextIsHonouredBeforeDialling(t *testing.T) {
	server := newFakeServer(t, fakeOptions{})
	mailer := newTestMailer(t, server, SMTPConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := mailer.Send(ctx, testMessage()); !errors.Is(err, context.Canceled) {
		t.Fatalf("应当原样返回 context 的错误，实际 %v", err)
	}
	if server.connectionCount() != 0 {
		t.Error("请求已经取消，不该再连出去")
	}
}

// RFC 5321 给一行的上限是 1000 字节。base64 折行要自己来，
// 漏掉的话一封长信就是一行几万字节。
func TestBodyLinesStayWithinTheLimit(t *testing.T) {
	server := newFakeServer(t, fakeOptions{})
	mailer := newTestMailer(t, server, SMTPConfig{})

	message := testMessage()
	message.Text = strings.Repeat("这是一段足够长的中文正文，用来把 base64 撑到需要折行的长度。\n", 200)

	if err := mailer.Send(context.Background(), message); err != nil {
		t.Fatalf("发送失败：%v", err)
	}

	raw := server.lastMessage(t)
	longest := 0
	wrapped := false
	for _, line := range strings.Split(raw, "\n") {
		longest = max(longest, len(line))
		if len(line) == base64LineWidth {
			wrapped = true
		}
	}
	if longest > 998 {
		t.Errorf("有行超过 RFC 5321 的上限，最长 %d 字节", longest)
	}
	if !wrapped {
		t.Error("这么长的正文应当发生折行")
	}

	_, body := parseMessage(t, raw)
	if body != message.Text {
		t.Error("折行之后正文没能原样还原")
	}
}
