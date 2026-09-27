package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

type fakeSessionStore struct {
	sessions map[string]model.Session
	users    map[string]model.User
	lookups  []string
	failWith error
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{sessions: map[string]model.Session{}, users: map[string]model.User{}}
}

func (f *fakeSessionStore) CreateSession(_ context.Context, session *model.Session) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.sessions[session.TokenHash] = *session
	return nil
}

func (f *fakeSessionStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*model.Session, error) {
	f.lookups = append(f.lookups, tokenHash)
	if f.failWith != nil {
		return nil, f.failWith
	}
	session, ok := f.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &session, nil
}

func (f *fakeSessionStore) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	user, ok := f.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &user, nil
}

func (f *fakeSessionStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	if f.failWith != nil {
		return f.failWith
	}
	delete(f.sessions, tokenHash)
	return nil
}

var _ SessionStore = (*fakeSessionStore)(nil)

func newTestSession(store SessionStore) *Session {
	sessions := NewSession(store)
	sessions.now = func() time.Time { return testNow }
	return sessions
}

func activeUser(status string) model.User {
	return model.User{
		ID:     "u1",
		Email:  "anxin@example.com",
		Role:   model.RoleMember,
		Status: status,
	}
}

func TestStartStoresHashNotToken(t *testing.T) {
	store := newFakeSessionStore()
	sessions := newTestSession(store)

	token, _, err := sessions.Start(context.Background(), "u1", "Mozilla/5.0", "203.0.113.7")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}

	stored, ok := store.sessions[auth.TokenHash(token)]
	if !ok {
		t.Fatal("会话没有按令牌哈希入库")
	}
	if stored.TokenHash == token {
		t.Error("库里存的是令牌原文")
	}
	if stored.UserID != "u1" || stored.IP != "203.0.113.7" || stored.UserAgent != "Mozilla/5.0" {
		t.Errorf("会话字段不符：%+v", stored)
	}
	if !stored.ExpiresAt.Equal(testNow.Add(SessionTTL)) {
		t.Errorf("有效期应为 %s，实际 %s", testNow.Add(SessionTTL), stored.ExpiresAt)
	}
	if !stored.CreatedAt.Equal(testNow) {
		t.Errorf("创建时间应取时钟，实际 %s", stored.CreatedAt)
	}
	if len(stored.ID) != 32 {
		t.Errorf("会话 id 应为 32 个字符，实际 %d：%q", len(stored.ID), stored.ID)
	}
}

func TestStartReturnsDistinctTokens(t *testing.T) {
	store := newFakeSessionStore()
	sessions := newTestSession(store)

	first, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	second, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	if first == second {
		t.Error("同一用户两次登录应得到不同的令牌")
	}
	if len(store.sessions) != 2 {
		t.Errorf("两次登录应留下两个会话，实际 %d", len(store.sessions))
	}
}

func TestAuthenticateAcceptsLiveSession(t *testing.T) {
	store := newFakeSessionStore()
	store.users["u1"] = activeUser(model.UserStatusActive)
	sessions := newTestSession(store)

	token, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}

	account, err := sessions.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("有效会话应通过：%v", err)
	}
	if account.User.ID != "u1" || account.User.Email != "anxin@example.com" {
		t.Errorf("带回来的账号不对：%+v", account.User)
	}
	if account.Session.TokenHash != auth.TokenHash(token) {
		t.Error("带回来的会话不是这一条")
	}
}

func TestAuthenticateLooksUpByHashNeverByToken(t *testing.T) {
	store := newFakeSessionStore()
	store.users["u1"] = activeUser(model.UserStatusActive)
	sessions := newTestSession(store)

	token, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	store.lookups = nil

	if _, err := sessions.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("有效会话应通过：%v", err)
	}
	if len(store.lookups) != 1 {
		t.Fatalf("应只查一次会话，实际 %d 次", len(store.lookups))
	}
	if store.lookups[0] == token {
		t.Error("拿令牌原文去查库了")
	}
	if store.lookups[0] != auth.TokenHash(token) {
		t.Errorf("查库用的不是令牌哈希：%q", store.lookups[0])
	}
}

func TestAuthenticateRejectsEmptyTokenWithoutTouchingStore(t *testing.T) {
	store := newFakeSessionStore()
	sessions := newTestSession(store)

	if _, err := sessions.Authenticate(context.Background(), ""); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("空令牌应判为无效，实际 %v", err)
	}
	if len(store.lookups) != 0 {
		t.Errorf("空令牌不该查库，实际查了 %d 次", len(store.lookups))
	}
}

func TestAuthenticateTreatsExpiryBoundaryAsExpired(t *testing.T) {
	store := newFakeSessionStore()
	store.users["u1"] = activeUser(model.UserStatusActive)
	sessions := newTestSession(store)

	token, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	hash := auth.TokenHash(token)

	atBoundary := store.sessions[hash]
	atBoundary.ExpiresAt = testNow
	store.sessions[hash] = atBoundary
	if _, err := sessions.Authenticate(context.Background(), token); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("恰好到期应判为无效，实际 %v", err)
	}

	justAlive := store.sessions[hash]
	justAlive.ExpiresAt = testNow.Add(time.Nanosecond)
	store.sessions[hash] = justAlive
	if _, err := sessions.Authenticate(context.Background(), token); err != nil {
		t.Errorf("还差 1 纳秒才到期，应通过，实际 %v", err)
	}
}

func TestAuthenticateRejectsEveryUnusableSessionAlike(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeSessionStore) string
	}{
		{
			name: "令牌查不到",
			setup: func(f *fakeSessionStore) string {
				return "not-a-real-token"
			},
		},
		{
			name: "用户已被删除",
			setup: func(f *fakeSessionStore) string {
				token, _, _ := newTestSession(f).Start(context.Background(), "gone", "", "")
				return token
			},
		},
		{
			name: "账号被停用",
			setup: func(f *fakeSessionStore) string {
				f.users["u1"] = activeUser(model.UserStatusDisabled)
				token, _, _ := newTestSession(f).Start(context.Background(), "u1", "", "")
				return token
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeSessionStore()
			store.users["u1"] = activeUser(model.UserStatusActive)
			token := tc.setup(store)

			if _, err := newTestSession(store).Authenticate(context.Background(), token); !errors.Is(err, ErrSessionInvalid) {
				t.Fatalf("应判为会话无效，实际 %v", err)
			}
		})
	}
}

// 数据库连不上不是「会话无效」。吞成 ErrSessionInvalid 会让一次故障看起来
// 像所有人同时被登出，而且中间件会回 401 而不是 500。
func TestAuthenticatePropagatesStoreFailure(t *testing.T) {
	store := newFakeSessionStore()
	store.failWith = errors.New("数据库连接失败")
	sessions := newTestSession(store)

	_, err := sessions.Authenticate(context.Background(), "some-token")
	if err == nil {
		t.Fatal("仓储故障应往上传")
	}
	if errors.Is(err, ErrSessionInvalid) {
		t.Errorf("仓储故障不该被当成会话无效：%v", err)
	}
	if !strings.Contains(err.Error(), "数据库连接失败") {
		t.Errorf("错误应保留原始信息：%v", err)
	}
}

func TestEndRemovesSessionAndIsIdempotent(t *testing.T) {
	store := newFakeSessionStore()
	store.users["u1"] = activeUser(model.UserStatusActive)
	sessions := newTestSession(store)

	token, _, err := sessions.Start(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}

	if err := sessions.End(context.Background(), token); err != nil {
		t.Fatalf("退出登录失败：%v", err)
	}
	if len(store.sessions) != 0 {
		t.Error("会话没有被删掉")
	}
	if err := sessions.End(context.Background(), token); err != nil {
		t.Errorf("重复退出不该报错：%v", err)
	}
	if _, err := sessions.Authenticate(context.Background(), token); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("退出后原令牌应失效，实际 %v", err)
	}
}

func TestEndWithoutTokenDoesNotTouchStore(t *testing.T) {
	store := newFakeSessionStore()
	sessions := newTestSession(store)

	if err := sessions.End(context.Background(), ""); err != nil {
		t.Fatalf("空令牌退出不该报错：%v", err)
	}
	if len(store.lookups) != 0 {
		t.Error("空令牌退出不该访问仓储")
	}
}
