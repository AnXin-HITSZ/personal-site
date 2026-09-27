package main

import (
	"context"
	"errors"
	"testing"

	"anxin-hitsz.com/backend/internal/auth"
)

// 默认值是空串，普通启动才会走「起服务」那条路。这个断言挡住的是
// 有人手滑把默认值写成别的、于是每次启动都去改一遍管理员口令。
func TestCreateAdminFlagDefaultsToOff(t *testing.T) {
	if *createAdminEmail != "" {
		t.Fatalf("-create-admin 默认值必须是空，实际 %q", *createAdminEmail)
	}
}

// 故意传 nil 数据库：邮箱不合法时必须在碰数据库之前就返回，
// 一旦顺序反了，这里会以一个空指针 panic 报警——这正是这条测试要钉住的位置关系。
func TestRunCreateAdminRejectsInvalidEmailBeforeTouchingDatabase(t *testing.T) {
	err := runCreateAdmin(context.Background(), nil, "不是邮箱")
	if !errors.Is(err, auth.ErrEmailInvalid) {
		t.Fatalf("应返回 ErrEmailInvalid，实际 %v", err)
	}
}
