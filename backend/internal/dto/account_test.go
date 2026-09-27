package dto

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

func sampleUser() model.User {
	verified := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	return model.User{
		ID:              "0123456789abcdef0123456789abcdef",
		Email:           "anxin@example.com",
		PasswordHash:    "$2a$12$verysecretverysecretverysecretverysecretverysecret",
		Role:            model.RoleMember,
		Status:          model.UserStatusDisabled,
		EmailVerifiedAt: &verified,
		CreatedAt:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func keysOf(t *testing.T, value any) []string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("反序列化失败：%v", err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestAccountViewExposesExactlyTheIntendedFields(t *testing.T) {
	got := keysOf(t, NewAccountView(sampleUser()))
	want := []string{"createdAt", "email", "emailVerified", "id", "role"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("字段集合不符：\n实际 %v\n期望 %v", got, want)
	}
}

// 手滑把 model.User 直接塞进响应里，口令哈希就出去了。这里用序列化结果兜底，
// 字段名怎么改都拦得住。
func TestAccountViewNeverLeaksSecrets(t *testing.T) {
	encoded, err := json.Marshal(NewAccountView(sampleUser()))
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	body := string(encoded)

	for _, forbidden := range []string{"$2a$", "passwordHash", "password_hash", "updatedAt", "disabled"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("响应里出现了不该出现的内容 %q：%s", forbidden, body)
		}
	}
}

func TestAccountViewDerivesEmailVerifiedFromTimestamp(t *testing.T) {
	user := sampleUser()
	if !NewAccountView(user).EmailVerified {
		t.Error("email_verified_at 有值时应为 true")
	}

	user.EmailVerifiedAt = nil
	if NewAccountView(user).EmailVerified {
		t.Error("email_verified_at 为空时应为 false")
	}
}

func TestSessionViewShapeAndTimeZone(t *testing.T) {
	expires := time.Date(2026, 10, 27, 4, 0, 0, 0, time.FixedZone("CST", 8*3600))
	view := NewSessionView(sampleUser(), expires)

	got := keysOf(t, view)
	if strings.Join(got, ",") != "account,expiresAt" {
		t.Errorf("字段集合不符：%v", got)
	}
	if !view.ExpiresAt.Equal(expires) {
		t.Error("到期时间被改动了")
	}
	if view.ExpiresAt.Location() != time.UTC {
		t.Errorf("时间应统一成 UTC 输出，实际 %s", view.ExpiresAt.Location())
	}
	if !view.Account.CreatedAt.Equal(sampleUser().CreatedAt) {
		t.Error("账号创建时间被改动了")
	}
}
