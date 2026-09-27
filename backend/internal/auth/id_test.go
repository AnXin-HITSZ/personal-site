package auth

import (
	"strings"
	"testing"
)

func TestNewIDFitsPrimaryKeyColumn(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("生成主键失败：%v", err)
		}
		if len(id) != 32 {
			t.Fatalf("主键应为 32 个字符，实际 %d：%q", len(id), id)
		}
		if strings.ContainsAny(id, "GHIJKLMNOPQRSTUVWXYZghijklmnopqrstuvwxyz-_") {
			t.Fatalf("主键不是十六进制：%q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("第 %d 次出现重复主键", i)
		}
		seen[id] = struct{}{}
	}
}

func TestNewIDDiffersFromToken(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("生成令牌失败：%v", err)
	}
	id, err := NewID()
	if err != nil {
		t.Fatalf("生成主键失败：%v", err)
	}
	if len(id) >= len(token) {
		t.Errorf("主键比令牌还长，说明用错了函数：主键 %d 字符，令牌 %d 字符", len(id), len(token))
	}
}
