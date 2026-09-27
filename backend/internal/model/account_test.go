package model

import (
	"slices"
	"sort"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// 迁移是表的唯一来源，应用启动不建表。模型和迁移一旦对不上，错的永远是模型，
// 而且不会报错——读会得到零值，写会静默丢字段。这个测试把两边钉在一起。
func TestModelsMatchMigrationColumns(t *testing.T) {
	cases := []struct {
		value   any
		table   string
		columns []string
	}{
		{
			&User{}, "users",
			[]string{"id", "email", "password_hash", "role", "status", "email_verified_at", "created_at", "updated_at"},
		},
		{
			&Session{}, "sessions",
			[]string{"id", "user_id", "token_hash", "user_agent", "ip", "expires_at", "created_at"},
		},
		{
			&AuthToken{}, "auth_tokens",
			[]string{"id", "user_id", "kind", "token_hash", "expires_at", "used_at", "created_at"},
		},
	}

	cache := &sync.Map{}
	for _, tc := range cases {
		parsed, err := schema.Parse(tc.value, cache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("%T 解析失败：%v", tc.value, err)
		}
		if parsed.Table != tc.table {
			t.Errorf("表名不符：模型给出 %q，迁移里是 %q", parsed.Table, tc.table)
		}

		got := make([]string, 0, len(parsed.Fields))
		for _, field := range parsed.Fields {
			got = append(got, field.DBName)
		}
		want := slices.Clone(tc.columns)
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s 的列不符：\n模型 %v\n迁移 %v", tc.table, got, want)
		}
	}
}
