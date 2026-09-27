package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/mail"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

// 种子的哈希在包初始化时算一次：每个用例都跑一遍 cost 12 太慢。
const (
	seedPassword = "correct horse battery staple"
	webPassword  = "a-much-longer-replacement"
)

var seededHash = mustHash(seedPassword)

func mustHash(password string) string {
	hash, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}
	return hash
}

type clock struct{ at time.Time }

func (c *clock) now() time.Time          { return c.at }
func (c *clock) advance(d time.Duration) { c.at = c.at.Add(d) }

type fakeMailer struct {
	sent []mail.Message
	err  error
}

func (m *fakeMailer) Send(_ context.Context, message mail.Message) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, message)
	return nil
}

// 令牌从正文里捞出来，再拿它去跑真实流程——这样测试验证的是「用户点开链接
// 能不能走通」，而不是「某个内部变量等于几」。
func tokenFromLastMail(t *testing.T, mailer *fakeMailer) string {
	t.Helper()
	if len(mailer.sent) == 0 {
		t.Fatal("没有发出任何邮件")
	}
	body := mailer.sent[len(mailer.sent)-1].Text
	index := strings.Index(body, "?token=")
	if index < 0 {
		t.Fatalf("邮件正文里没有带令牌的链接：%s", body)
	}
	rest := body[index+len("?token="):]
	if end := strings.IndexAny(rest, "\r\n \t"); end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		t.Fatal("链接里的令牌是空的")
	}
	return rest
}

type fakeAccountStore struct {
	users    map[string]*model.User
	byEmail  map[string]string
	tokens   map[string]*model.AuthToken
	sessions map[string]*model.Session
	offline  error
}

func newFakeStore() *fakeAccountStore {
	return &fakeAccountStore{
		users:    map[string]*model.User{},
		byEmail:  map[string]string{},
		tokens:   map[string]*model.AuthToken{},
		sessions: map[string]*model.Session{},
	}
}

func (s *fakeAccountStore) guard() error { return s.offline }

func (s *fakeAccountStore) seedUser(email, role, status string, verified bool) *model.User {
	user := &model.User{
		ID:           fmt.Sprintf("u%d", len(s.users)+1),
		Email:        email,
		PasswordHash: seededHash,
		Role:         role,
		Status:       status,
		CreatedAt:    testNow,
		UpdatedAt:    testNow,
	}
	if verified {
		at := testNow
		user.EmailVerifiedAt = &at
	}
	s.users[user.ID] = user
	s.byEmail[email] = user.ID
	return user
}

func (s *fakeAccountStore) CreateUser(_ context.Context, user *model.User) error {
	if err := s.guard(); err != nil {
		return err
	}
	if _, taken := s.byEmail[user.Email]; taken {
		return repository.ErrEmailTaken
	}
	stored := *user
	s.users[user.ID] = &stored
	s.byEmail[user.Email] = user.ID
	return nil
}

func (s *fakeAccountStore) GetUserByEmail(_ context.Context, email string) (*model.User, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	id, ok := s.byEmail[email]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return s.users[id], nil
}

func (s *fakeAccountStore) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	user, ok := s.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return user, nil
}

func (s *fakeAccountStore) MarkEmailVerified(_ context.Context, userID string, at time.Time) error {
	if err := s.guard(); err != nil {
		return err
	}
	user, ok := s.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	if user.EmailVerifiedAt == nil {
		verified := at
		user.EmailVerifiedAt = &verified
	}
	return nil
}

func (s *fakeAccountStore) UpdatePasswordHash(_ context.Context, userID, hash string) error {
	if err := s.guard(); err != nil {
		return err
	}
	user, ok := s.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	user.PasswordHash = hash
	return nil
}

func (s *fakeAccountStore) CreateAuthToken(_ context.Context, token *model.AuthToken) error {
	if err := s.guard(); err != nil {
		return err
	}
	stored := *token
	s.tokens[token.TokenHash] = &stored
	return nil
}

func (s *fakeAccountStore) ReplaceAuthToken(_ context.Context, token *model.AuthToken) error {
	if err := s.guard(); err != nil {
		return err
	}
	for hash, existing := range s.tokens {
		if existing.UserID == token.UserID && existing.Kind == token.Kind {
			delete(s.tokens, hash)
		}
	}
	stored := *token
	s.tokens[token.TokenHash] = &stored
	return nil
}

func (s *fakeAccountStore) ConsumeAuthToken(_ context.Context, kind, tokenHash string, now time.Time) (*model.AuthToken, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	token, ok := s.tokens[tokenHash]
	if !ok || token.Kind != kind || token.UsedAt != nil || !token.ExpiresAt.After(now) {
		return nil, repository.ErrNotFound
	}
	used := now
	token.UsedAt = &used
	stored := *token
	return &stored, nil
}

func (s *fakeAccountStore) CreateSession(_ context.Context, session *model.Session) error {
	if err := s.guard(); err != nil {
		return err
	}
	stored := *session
	s.sessions[session.ID] = &stored
	return nil
}

func (s *fakeAccountStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*model.Session, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	for _, session := range s.sessions {
		if session.TokenHash == tokenHash {
			return session, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (s *fakeAccountStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	if err := s.guard(); err != nil {
		return err
	}
	for id, session := range s.sessions {
		if session.TokenHash == tokenHash {
			delete(s.sessions, id)
			return nil
		}
	}
	return nil
}

func (s *fakeAccountStore) DeleteSessionsByUser(_ context.Context, userID, exceptID string) (int64, error) {
	if err := s.guard(); err != nil {
		return 0, err
	}
	var deleted int64
	for id, session := range s.sessions {
		if session.UserID == userID && id != exceptID {
			delete(s.sessions, id)
			deleted++
		}
	}
	return deleted, nil
}

func (s *fakeAccountStore) ListSessionsByUser(_ context.Context, userID string) ([]model.Session, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	var found []model.Session
	for _, session := range s.sessions {
		if session.UserID == userID {
			found = append(found, *session)
		}
	}
	return found, nil
}

func (s *fakeAccountStore) DeleteSession(_ context.Context, userID, sessionID string) (int64, error) {
	if err := s.guard(); err != nil {
		return 0, err
	}
	session, ok := s.sessions[sessionID]
	if !ok || session.UserID != userID {
		return 0, nil
	}
	delete(s.sessions, sessionID)
	return 1, nil
}

type accountFixture struct {
	store    *fakeAccountStore
	mailer   *fakeMailer
	clock    *clock
	sessions *Session
	account  *Account
}

func newAccountFixture() *accountFixture {
	store := newFakeStore()
	mailer := &fakeMailer{}
	at := &clock{at: testNow}

	sessions := NewSession(store)
	sessions.now = at.now
	account := NewAccount(store, sessions, mailer, "https://anxin-hitsz.com")
	account.now = at.now

	return &accountFixture{store: store, mailer: mailer, clock: at, sessions: sessions, account: account}
}

func (f *accountFixture) lastMessage(t *testing.T) mail.Message {
	t.Helper()
	if len(f.mailer.sent) == 0 {
		t.Fatal("没有发出任何邮件")
	}
	return f.mailer.sent[len(f.mailer.sent)-1]
}

func TestRegisterCreatesUnverifiedMember(t *testing.T) {
	f := newAccountFixture()

	if err := f.account.Register(context.Background(), " Owner@Example.COM ", seedPassword); err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	if len(f.store.users) != 1 {
		t.Fatalf("应有一个账号，实际 %d 个", len(f.store.users))
	}
	var user *model.User
	for _, candidate := range f.store.users {
		user = candidate
	}

	if user.Email != "owner@example.com" {
		t.Errorf("邮箱应已归一化，实际 %q", user.Email)
	}
	if user.Role != model.RoleMember {
		t.Errorf("注册只能产生 member，实际 %q", user.Role)
	}
	if user.Status != model.UserStatusActive {
		t.Errorf("新账号应为 active，实际 %q", user.Status)
	}
	if user.EmailVerifiedAt != nil {
		t.Error("新账号不该是已验证状态")
	}
	if user.PasswordHash == seedPassword || !auth.VerifyPassword(user.PasswordHash, seedPassword) {
		t.Error("库里应存哈希，且能校验回来")
	}
}

// 令牌在库里存的是哈希：库被读了也不等于能拿着链接去验证别人的邮箱。
func TestRegisterStoresTokenHashAndMailsUsableLink(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.Register(context.Background(), "owner@example.com", seedPassword); err != nil {
		t.Fatalf("注册失败：%v", err)
	}

	token := tokenFromLastMail(t, f.mailer)
	if _, plainStored := f.store.tokens[token]; plainStored {
		t.Fatal("令牌不该以明文入库")
	}
	if _, hashed := f.store.tokens[auth.TokenHash(token)]; !hashed {
		t.Fatal("库里应存的是令牌哈希")
	}

	if err := f.account.VerifyEmail(context.Background(), token); err != nil {
		t.Fatalf("邮件里的令牌应该能用：%v", err)
	}
}

// 邮箱被占用时不能报错——报了错，这个接口就成了「查这个邮箱注册过没有」。
func TestRegisterWithTakenEmailLooksLikeSuccess(t *testing.T) {
	f := newAccountFixture()
	existing := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	if err := f.account.Register(context.Background(), "owner@example.com", seedPassword); err != nil {
		t.Fatalf("不该报错，实际 %v", err)
	}
	if len(f.store.users) != 1 {
		t.Errorf("不该创建第二个账号，实际 %d 个", len(f.store.users))
	}
	if existing.PasswordHash != seededHash {
		t.Error("不该动到已有账号的口令")
	}

	// 但要给真正的主人提个醒。
	message := f.lastMessage(t)
	if message.To != "owner@example.com" {
		t.Errorf("提醒应发给邮箱主人，实际 %q", message.To)
	}
	if strings.Contains(message.Text, "?token=") {
		t.Error("这封信不该带任何令牌")
	}
}

func TestRegisterRejectsWeakInput(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
		want     error
	}{
		{name: "邮箱格式不对", email: "不是邮箱", password: seedPassword, want: auth.ErrEmailInvalid},
		{name: "口令过短", email: "owner@example.com", password: "short", want: auth.ErrPasswordTooShort},
		{name: "口令和邮箱相同", email: "owner@example.com", password: "owner@example.com", want: auth.ErrPasswordSameMail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			err := f.account.Register(context.Background(), tc.email, tc.password)
			if !errors.Is(err, tc.want) {
				t.Fatalf("应报 %v，实际 %v", tc.want, err)
			}
			if len(f.store.users) != 0 {
				t.Error("校验没过就不该写库")
			}
			if len(f.mailer.sent) != 0 {
				t.Error("校验没过就不该发信")
			}
		})
	}
}

func TestVerifyEmailTokenIsSingleUse(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.Register(context.Background(), "owner@example.com", seedPassword); err != nil {
		t.Fatal(err)
	}
	token := tokenFromLastMail(t, f.mailer)

	if err := f.account.VerifyEmail(context.Background(), token); err != nil {
		t.Fatalf("第一次应该成功：%v", err)
	}
	if err := f.account.VerifyEmail(context.Background(), token); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("同一个令牌第二次应作废，实际 %v", err)
	}
}

func TestVerifyEmailRejectsExpiredToken(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.Register(context.Background(), "owner@example.com", seedPassword); err != nil {
		t.Fatal(err)
	}
	token := tokenFromLastMail(t, f.mailer)

	f.clock.advance(EmailVerifyTTL)
	if err := f.account.VerifyEmail(context.Background(), token); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("正好到期的令牌应作废，实际 %v", err)
	}
}

func TestVerifyEmailRejectsEmptyToken(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.VerifyEmail(context.Background(), ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("空令牌应报无效，实际 %v", err)
	}
}

// 连点五次「重新发送」，不能留下五个同时有效的链接。
func TestResendVerificationInvalidatesPreviousToken(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.Register(context.Background(), "owner@example.com", seedPassword); err != nil {
		t.Fatal(err)
	}
	first := tokenFromLastMail(t, f.mailer)

	if err := f.account.ResendVerification(context.Background(), "owner@example.com"); err != nil {
		t.Fatalf("重发失败：%v", err)
	}
	second := tokenFromLastMail(t, f.mailer)
	if second == first {
		t.Fatal("重发应产生新令牌")
	}

	if err := f.account.VerifyEmail(context.Background(), first); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("旧令牌应已失效，实际 %v", err)
	}
	if err := f.account.VerifyEmail(context.Background(), second); err != nil {
		t.Errorf("新令牌应该能用：%v", err)
	}
}

func TestResendVerificationIsSilentWhenNothingToDo(t *testing.T) {
	cases := []struct {
		name string
		seed func(*accountFixture)
	}{
		{name: "邮箱不存在"},
		{name: "邮箱已经验证过", seed: func(f *accountFixture) {
			f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			if tc.seed != nil {
				tc.seed(f)
			}
			if err := f.account.ResendVerification(context.Background(), "owner@example.com"); err != nil {
				t.Fatalf("不该报错，实际 %v", err)
			}
			if len(f.mailer.sent) != 0 {
				t.Errorf("不该发信，实际发了 %d 封", len(f.mailer.sent))
			}
		})
	}
}

func TestLoginStartsSession(t *testing.T) {
	f := newAccountFixture()
	user := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	result, err := f.account.Login(context.Background(), "Owner@Example.com", seedPassword, "Mozilla/5.0", "203.0.113.7")
	if err != nil {
		t.Fatalf("登录失败：%v", err)
	}
	if result.User.ID != user.ID {
		t.Errorf("返回的账号不对：%s", result.User.ID)
	}
	if result.ExpiresAt != testNow.Add(SessionTTL) {
		t.Errorf("有效期应为 %v，实际 %v", testNow.Add(SessionTTL), result.ExpiresAt)
	}

	stored, ok := f.store.sessions[sessionIDByToken(f, result.Token)]
	if !ok {
		t.Fatal("会话没有入库")
	}
	if stored.IP != "203.0.113.7" || stored.UserAgent != "Mozilla/5.0" {
		t.Errorf("诊断信息没记下来：%+v", stored)
	}
}

func sessionIDByToken(f *accountFixture, token string) string {
	for id, session := range f.store.sessions {
		if session.TokenHash == auth.TokenHash(token) {
			return id
		}
	}
	return ""
}

func TestLoginRejectsBadInput(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
		want     error
	}{
		{name: "口令不对", email: "owner@example.com", password: "wrong-password-here", want: ErrCredentialsInvalid},
		{name: "邮箱不存在", email: "nobody@example.com", password: seedPassword, want: ErrCredentialsInvalid},
		{name: "邮箱格式不对", email: "不是邮箱", password: seedPassword, want: ErrCredentialsInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

			_, err := f.account.Login(context.Background(), tc.email, tc.password, "UA", "1.2.3.4")
			if !errors.Is(err, tc.want) {
				t.Fatalf("应报 %v，实际 %v", tc.want, err)
			}
			if len(f.store.sessions) != 0 {
				t.Error("登录失败不该留下会话")
			}
		})
	}
}

// 这条测的是耗时，不是返回值。把替身哈希的比较删掉，未知邮箱会从 250ms
// 掉到几微秒，这个接口就成了一个「查邮箱注册过没有」的接口。
// 阈值给得很宽：要拦的是防御整个消失，不是去测 bcrypt 到底跑了多久。
func TestLoginPaysHashCostForUnknownEmail(t *testing.T) {
	f := newAccountFixture()
	f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	startedAt := time.Now()
	_, err := f.account.Login(context.Background(), "nobody@example.com", seedPassword, "UA", "1.2.3.4")
	elapsed := time.Since(startedAt)

	if !errors.Is(err, ErrCredentialsInvalid) {
		t.Fatalf("应报凭据无效，实际 %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("未知邮箱只花了 %v，说明这次登录没有付出哈希代价", elapsed)
	}
}

// 未验证、已停用这两个错误只会发给口令已经输对的人。口令错的时候必须还是
// 「凭据无效」，否则试错的人能靠响应分辨出哪些邮箱真实存在。
func TestLoginHidesAccountStateUntilPasswordMatches(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		verified bool
	}{
		{name: "未验证", status: model.UserStatusActive, verified: false},
		{name: "已停用", status: model.UserStatusDisabled, verified: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			f.store.seedUser("owner@example.com", model.RoleMember, tc.status, tc.verified)

			_, err := f.account.Login(context.Background(), "owner@example.com", "wrong-password-here", "UA", "1.2.3.4")
			if !errors.Is(err, ErrCredentialsInvalid) {
				t.Errorf("口令不对时应报凭据无效，实际 %v", err)
			}
		})
	}
}

func TestLoginRejectsVerifiedAndDisabledStates(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		verified bool
		want     error
	}{
		{name: "邮箱未验证", status: model.UserStatusActive, verified: false, want: ErrEmailNotVerified},
		{name: "账号已停用", status: model.UserStatusDisabled, verified: true, want: ErrAccountDisabled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			f.store.seedUser("owner@example.com", model.RoleMember, tc.status, tc.verified)

			_, err := f.account.Login(context.Background(), "owner@example.com", seedPassword, "UA", "1.2.3.4")
			if !errors.Is(err, tc.want) {
				t.Fatalf("应报 %v，实际 %v", tc.want, err)
			}
			if len(f.store.sessions) != 0 {
				t.Error("没登进来就不该留下会话")
			}
		})
	}
}

func TestLogoutEndsSessionAndIsIdempotent(t *testing.T) {
	f := newAccountFixture()
	f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	result, err := f.account.Login(context.Background(), "owner@example.com", seedPassword, "UA", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.account.Logout(context.Background(), result.Token); err != nil {
		t.Fatalf("退出失败：%v", err)
	}
	if len(f.store.sessions) != 0 {
		t.Errorf("会话应已删除，实际还剩 %d 个", len(f.store.sessions))
	}
	if err := f.account.Logout(context.Background(), result.Token); err != nil {
		t.Errorf("重复退出不该报错，实际 %v", err)
	}
	if err := f.account.Logout(context.Background(), ""); err != nil {
		t.Errorf("空令牌不该报错，实际 %v", err)
	}
}

func TestRequestPasswordResetBehaviour(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		wantMails int
	}{
		{name: "正常账号", status: model.UserStatusActive, wantMails: 1},
		{name: "已停用账号", status: model.UserStatusDisabled, wantMails: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			f.store.seedUser("owner@example.com", model.RoleMember, tc.status, true)

			if err := f.account.RequestPasswordReset(context.Background(), "owner@example.com"); err != nil {
				t.Fatalf("不该报错，实际 %v", err)
			}
			if len(f.mailer.sent) != tc.wantMails {
				t.Fatalf("应发 %d 封，实际 %d 封", tc.wantMails, len(f.mailer.sent))
			}
			if tc.wantMails > 0 {
				token := tokenFromLastMail(t, f.mailer)
				if _, ok := f.store.tokens[auth.TokenHash(token)]; !ok {
					t.Error("邮件里的令牌应该已经入库")
				}
			}
		})
	}
}

func TestRequestPasswordResetIsSilentForUnknownEmail(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.RequestPasswordReset(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("不该报错，实际 %v", err)
	}
	if len(f.mailer.sent) != 0 {
		t.Error("未知邮箱不该发信")
	}
}

func TestResetPasswordConsumesTokenAndKillsEverySession(t *testing.T) {
	f := newAccountFixture()
	user := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	if _, _, err := f.sessions.Start(context.Background(), user.ID, "笔记本", "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.sessions.Start(context.Background(), user.ID, "手机", "198.51.100.9"); err != nil {
		t.Fatal(err)
	}

	if err := f.account.RequestPasswordReset(context.Background(), "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	token := tokenFromLastMail(t, f.mailer)

	if err := f.account.ResetPassword(context.Background(), token, webPassword); err != nil {
		t.Fatalf("重置失败：%v", err)
	}

	if !auth.VerifyPassword(f.store.users[user.ID].PasswordHash, webPassword) {
		t.Error("新口令应该能用")
	}
	if len(f.store.sessions) != 0 {
		t.Errorf("重置口令要把所有设备踢下线，实际还剩 %d 个会话", len(f.store.sessions))
	}
	// 令牌是一次性的：邮件被转发出去也重放不了。
	if err := f.account.ResetPassword(context.Background(), token, "another-long-password"); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("同一个令牌第二次应作废，实际 %v", err)
	}
}

// 口令太短是最常见的手滑，不能让一次手滑就烧掉刚收到的链接。
func TestResetPasswordValidatesLengthBeforeSpendingToken(t *testing.T) {
	f := newAccountFixture()
	f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)
	if err := f.account.RequestPasswordReset(context.Background(), "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	token := tokenFromLastMail(t, f.mailer)

	if err := f.account.ResetPassword(context.Background(), token, "short"); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Fatalf("应报口令过短，实际 %v", err)
	}
	if err := f.account.ResetPassword(context.Background(), token, webPassword); err != nil {
		t.Errorf("链接应该还能用：%v", err)
	}
}

func TestResetPasswordRejectsUnknownToken(t *testing.T) {
	f := newAccountFixture()
	if err := f.account.ResetPassword(context.Background(), "从来没有过的令牌", webPassword); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("应报链接无效，实际 %v", err)
	}
}

func TestChangePasswordKeepsCurrentDeviceOnly(t *testing.T) {
	f := newAccountFixture()
	user := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

	current, _, err := f.sessions.Start(context.Background(), user.ID, "当前设备", "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.sessions.Start(context.Background(), user.ID, "别的设备", "198.51.100.9"); err != nil {
		t.Fatal(err)
	}
	currentID := sessionIDByToken(f, current)

	kicked, err := f.account.ChangePassword(context.Background(), user.ID, currentID, seedPassword, webPassword)
	if err != nil {
		t.Fatalf("改口令失败：%v", err)
	}
	if kicked != 1 {
		t.Errorf("应踢掉 1 个其它设备，实际 %d 个", kicked)
	}
	if _, stillThere := f.store.sessions[currentID]; !stillThere {
		t.Error("当前设备不该被踢下线")
	}
	if !auth.VerifyPassword(f.store.users[user.ID].PasswordHash, webPassword) {
		t.Error("新口令应该能用")
	}
}

func TestChangePasswordRejections(t *testing.T) {
	cases := []struct {
		name    string
		current string
		next    string
		want    error
	}{
		{name: "当前口令不对", current: "wrong-password-here", next: webPassword, want: ErrCurrentPasswordWrong},
		{name: "新口令太短", current: seedPassword, next: "short", want: auth.ErrPasswordTooShort},
		{name: "新口令和当前相同", current: seedPassword, next: seedPassword, want: ErrPasswordUnchanged},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			user := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)

			if _, err := f.account.ChangePassword(context.Background(), user.ID, "s1", tc.current, tc.next); !errors.Is(err, tc.want) {
				t.Fatalf("应报 %v，实际 %v", tc.want, err)
			}
			if f.store.users[user.ID].PasswordHash != seededHash {
				t.Error("失败时不该动到口令")
			}
		})
	}
}

// 拿到别人的会话 ID 也删不掉别人的会话：删除按 (userID, sessionID) 定位。
func TestRevokeSessionIsScopedToOwner(t *testing.T) {
	f := newAccountFixture()
	owner := f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)
	other := f.store.seedUser("other@example.com", model.RoleMember, model.UserStatusActive, true)

	victim, _, err := f.sessions.Start(context.Background(), other.ID, "别人的设备", "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	victimID := sessionIDByToken(f, victim)

	deleted, err := f.account.RevokeSession(context.Background(), owner.ID, victimID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Errorf("不该删掉别人的会话，实际删了 %d 个", deleted)
	}
	if _, stillThere := f.store.sessions[victimID]; !stillThere {
		t.Error("别人的会话应该还在")
	}

	deleted, err = f.account.RevokeSession(context.Background(), other.ID, victimID)
	if err != nil || deleted != 1 {
		t.Errorf("本人来删应成功，实际 deleted=%d err=%v", deleted, err)
	}
}

// 邮件发不出去不能变成 500：那等于告诉请求方「这个邮箱是存在的，
// 只是我们的邮件出了问题」。
func TestMailFailureDoesNotLeakAccountExistence(t *testing.T) {
	cases := []struct {
		name  string
		email string
		seed  bool
	}{
		{name: "邮箱存在", email: "owner@example.com", seed: true},
		{name: "邮箱不存在", email: "nobody@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountFixture()
			f.mailer.err = mail.ErrNotConfigured
			if tc.seed {
				f.store.seedUser("owner@example.com", model.RoleMember, model.UserStatusActive, true)
			}

			if err := f.account.RequestPasswordReset(context.Background(), tc.email); err != nil {
				t.Errorf("发信失败不该冒出来，实际 %v", err)
			}
			if err := f.account.Register(context.Background(), tc.email, seedPassword); err != nil {
				t.Errorf("注册时发信失败也不该冒出来，实际 %v", err)
			}
		})
	}
}
