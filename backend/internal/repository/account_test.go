package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"anxin-hitsz.com/backend/internal/model"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestCreateUserWritesToUsersTable(t *testing.T) {
	f := newFixture(t)
	user := &model.User{
		ID:           "0123456789abcdef0123456789abcdef",
		Email:        "anxin@example.com",
		PasswordHash: "$2a$12$abcdefghijklmnopqrstuv",
		Role:         model.RoleMember,
		Status:       model.UserStatusActive,
	}
	if err := f.repo.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	statement := f.rec.expect(t, "INSERT INTO `users`")
	for _, column := range []string{"`id`", "`email`", "`password_hash`", "`role`", "`status`", "`created_at`", "`updated_at`"} {
		if !strings.Contains(statement, column) {
			t.Errorf("INSERT 少了列 %s：\n%s", column, statement)
		}
	}
	// 角色和状态是紧跟其后的两个值：注册只写 member，未验证即 email_verified_at 为 NULL。
	if !strings.Contains(statement, "'member','active',NULL") {
		t.Errorf("注册应写死 member/active 且不带验证时间：\n%s", statement)
	}
}

// DryRun 会跳过执行，所以这里只看语句形状，看不到 ErrRecordNotFound→ErrNotFound
// 的映射——那条路径要靠真实数据库（或假驱动返回零行）才能覆盖。
func TestGetUserByEmailMatchesOnlyEmail(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.GetUserByEmail(context.Background(), "anxin@example.com"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "SELECT * FROM `users` WHERE email = 'anxin@example.com'")
}

func TestGetSessionByTokenHashUsesUniqueIndex(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.GetSessionByTokenHash(context.Background(), "deadbeef"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "SELECT * FROM `sessions` WHERE token_hash = 'deadbeef'")
}

func TestMarkEmailVerifiedSkipsAlreadyVerified(t *testing.T) {
	f := newFixture(t)
	if err := f.repo.MarkEmailVerified(context.Background(), "u1", testNow); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "UPDATE `users` SET", "`email_verified_at`=", "WHERE id = 'u1' AND email_verified_at IS NULL")
}

func TestUpdatePasswordHashTargetsOneUser(t *testing.T) {
	f := newFixture(t)
	if err := f.repo.UpdatePasswordHash(context.Background(), "u1", "$2a$12$new"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	statement := f.rec.expect(t, "UPDATE `users` SET", "WHERE id = 'u1'")
	if !strings.Contains(statement, "`password_hash`=") {
		t.Errorf("没有更新口令列：\n%s", statement)
	}
}

// MakeAdmin 是唯一能把 role 写成 admin 的地方。四个字段必须在同一条 UPDATE 里，
// 否则中间那一刻就是「已经是管理员、口令还是旧的」。
func TestMakeAdminWritesRoleAndCredentialsInOneStatement(t *testing.T) {
	f := newFixture(t)
	if err := f.repo.MakeAdmin(context.Background(), "u1", "$2a$12$new", testNow); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	if count := len(f.rec.all()); count != 1 {
		t.Fatalf("应只有一条语句，实际 %d 条：%v", count, f.rec.all())
	}

	statement := f.rec.expect(t, "UPDATE `users` SET", "WHERE id = 'u1'")
	for _, want := range []string{"`password_hash`=", "`role`=", "`status`=", "`email_verified_at`="} {
		if !strings.Contains(statement, want) {
			t.Errorf("缺少 %s：\n%s", want, statement)
		}
	}
	// role 和 status 的值直接钉在 SQL 里：写成 'member' 或 'disabled' 都不会报错，
	// 只会安静地把管理员建成一个登不进来的账号。
	if !strings.Contains(statement, "'admin'") || !strings.Contains(statement, "'active'") {
		t.Errorf("role 应为 admin、status 应为 active：\n%s", statement)
	}
}

func TestCreateSessionTruncatesDiagnosticColumns(t *testing.T) {
	f := newFixture(t)
	session := &model.Session{
		ID:        "s1",
		UserID:    "u1",
		TokenHash: strings.Repeat("a", 64),
		UserAgent: strings.Repeat("b", 300),
		IP:        "203.0.113.7",
		ExpiresAt: testNow.Add(30 * 24 * time.Hour),
	}
	if err := f.repo.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	if got := len([]rune(session.UserAgent)); got != maxUserAgentRunes {
		t.Errorf("user_agent 应被截到 %d 个字符，实际 %d", maxUserAgentRunes, got)
	}
	statement := f.rec.expect(t, "INSERT INTO `sessions`", "`user_agent`")
	f.rec.reject(t, strings.Repeat("b", maxUserAgentRunes+1))
	if !strings.Contains(statement, "`ip`") {
		t.Errorf("INSERT 少了 ip：\n%s", statement)
	}
}

func TestDeleteSessionIsScopedToOwner(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.DeleteSession(context.Background(), "u1", "s2"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "DELETE FROM `sessions`", "user_id = 'u1'", "id = 's2'")
}

func TestDeleteSessionsByUserSparesCurrentSession(t *testing.T) {
	f := newFixture(t)

	if _, err := f.repo.DeleteSessionsByUser(context.Background(), "u1", "current"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}
	f.rec.expect(t, "DELETE FROM `sessions`", "user_id = 'u1'", "id <> 'current'")

	f.rec.reset()
	if _, err := f.repo.DeleteSessionsByUser(context.Background(), "u1", ""); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}
	f.rec.expect(t, "DELETE FROM `sessions`", "user_id = 'u1'")
	f.rec.reject(t, "id <>")
}

func TestDeleteExpiredSessionsTargetsExpiry(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.DeleteExpiredSessions(context.Background(), testNow); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "DELETE FROM `sessions`", "expires_at <=", "2026-09-27 12:00:00")
}

func TestConsumeAuthTokenCarriesEveryGuard(t *testing.T) {
	f := newFixture(t)
	tokenHash := strings.Repeat("c", 64)

	if _, err := f.repo.ConsumeAuthToken(context.Background(), model.TokenPasswordReset, tokenHash, testNow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("影响行数为 0 时应报未找到，实际：%v", err)
	}

	f.rec.expect(t, "UPDATE `auth_tokens` SET",
		"`used_at`=",
		"token_hash = '"+tokenHash+"'",
		"kind = 'password_reset'",
		"used_at IS NULL",
		"expires_at >",
		"2026-09-27 12:00:00",
	)
}

func TestReplaceAuthTokenDeletesBeforeInserting(t *testing.T) {
	f := newFixture(t)
	token := &model.AuthToken{
		ID:        "t1",
		UserID:    "u1",
		Kind:      model.TokenEmailVerify,
		TokenHash: strings.Repeat("d", 64),
		ExpiresAt: testNow.Add(24 * time.Hour),
	}
	if err := f.repo.ReplaceAuthToken(context.Background(), token); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	deleted := f.rec.indexOf(t, "DELETE FROM `auth_tokens`")
	inserted := f.rec.indexOf(t, "INSERT INTO `auth_tokens`")
	if deleted > inserted {
		t.Errorf("应先删旧令牌再插新令牌，实际顺序：\n%s", strings.Join(f.rec.all(), "\n"))
	}
	f.rec.expect(t, "DELETE FROM `auth_tokens`", "user_id = 'u1'", "kind = 'email_verify'")
	if begins := f.conn.beginCount(); begins != 1 {
		t.Errorf("删除与插入应在同一个事务里，实际开了 %d 个事务", begins)
	}
}

func TestListSessionsOrdersByNewest(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.ListSessionsByUser(context.Background(), "u1"); err != nil {
		t.Fatalf("DryRun 下不该报错：%v", err)
	}

	f.rec.expect(t, "SELECT * FROM `sessions` WHERE user_id = 'u1' ORDER BY created_at desc")
}

func TestIsDuplicateEntrySeesThroughWrapping(t *testing.T) {
	duplicate := &mysql.MySQLError{Number: errnoDuplicateEntry, Message: "Duplicate entry 'a@b.com' for key 'users.uk_users_email'"}
	if !isDuplicateEntry(duplicate) {
		t.Error("1062 应被识别为重复")
	}
	if !isDuplicateEntry(fmt.Errorf("写入失败：%w", duplicate)) {
		t.Error("被包装过的 1062 也应被识别")
	}
	if isDuplicateEntry(&mysql.MySQLError{Number: 3819}) {
		t.Error("CHECK 约束冲突不该被当成重复")
	}
	if isDuplicateEntry(nil) {
		t.Error("nil 不该被当成重复")
	}
}

func TestTruncateRunesCountsCharacters(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"不超长原样返回", strings.Repeat("a", 10), 10},
		{"恰好等长不动", strings.Repeat("a", 20), 20},
		{"超长按字符截", strings.Repeat("a", 30), 20},
		{"汉字按字符不按字节", strings.Repeat("密", 30), 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.in, 20)
			if count := len([]rune(got)); count != tc.want {
				t.Errorf("应剩 %d 个字符，实际 %d：%q", tc.want, count, got)
			}
		})
	}
}

func TestTruncateRunesDropsInvalidUTF8(t *testing.T) {
	got := truncateRunes("正常\xff\xfe结尾", 100)
	if !strings.HasPrefix(got, "正常") || !strings.HasSuffix(got, "结尾") {
		t.Errorf("非法字节应被删掉、合法部分应保留，实际：%q", got)
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("不该留下替换符：%q", got)
	}
}

func TestTruncateRunesKeepsValidBoundary(t *testing.T) {
	got := truncateRunes(strings.Repeat("密", 30), 20)
	if got != strings.Repeat("密", 20) {
		t.Errorf("截断点落在了字符中间：%q", got)
	}
}
