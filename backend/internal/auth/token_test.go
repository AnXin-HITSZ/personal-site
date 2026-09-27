package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewTokenIsDecodableAndTokensAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		token, err := NewToken()
		if err != nil {
			t.Fatalf("生成令牌失败：%v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("令牌不是合法的无 padding base64url：%q（%v）", token, err)
		}
		if len(raw) != tokenBytes {
			t.Fatalf("令牌解码后应为 %d 字节，实际 %d", tokenBytes, len(raw))
		}
		if _, dup := seen[token]; dup {
			t.Fatalf("第 %d 次生成出现重复令牌", i)
		}
		seen[token] = struct{}{}
	}
}

// 令牌要放进 URL 查询串和 cookie 值，出现这些字符就得转义。
func TestNewTokenNeedsNoEscaping(t *testing.T) {
	for i := 0; i < 100; i++ {
		token, err := NewToken()
		if err != nil {
			t.Fatalf("生成令牌失败：%v", err)
		}
		if strings.ContainsAny(token, "+/=?#& ") {
			t.Fatalf("令牌含需转义字符：%q", token)
		}
	}
}

func TestTokenHashIsStableHexOf64(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("生成令牌失败：%v", err)
	}
	first := TokenHash(token)
	if first != TokenHash(token) {
		t.Error("同一令牌两次哈希应相同")
	}
	if len(first) != 64 {
		t.Errorf("sha256 的十六进制应为 64 字符，实际 %d", len(first))
	}
	if strings.ContainsAny(first, "GHIJKLMNOPQRSTUVWXYZghijklmnopqrstuvwxyz-_") {
		t.Errorf("不是十六进制：%s", first)
	}
}

func TestTokenHashHidesToken(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("生成令牌失败：%v", err)
	}
	hash := TokenHash(token)
	if strings.Contains(hash, token) {
		t.Error("哈希里出现了令牌原文")
	}
	other, err := NewToken()
	if err != nil {
		t.Fatalf("生成令牌失败：%v", err)
	}
	if hash == TokenHash(other) {
		t.Error("不同令牌的哈希应不同")
	}
}
