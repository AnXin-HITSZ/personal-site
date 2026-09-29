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

func TestNewShortIDShape(t *testing.T) {
	seen := make(map[string]struct{}, 2000)
	for i := 0; i < 2000; i++ {
		id, err := NewShortID()
		if err != nil {
			t.Fatalf("生成短主键失败：%v", err)
		}
		if len(id) != 8 {
			t.Fatalf("短主键应为 8 个字符，实际 %d：%q", len(id), id)
		}
		if strings.ContainsAny(id, "ilou") {
			t.Fatalf("短主键用到了字母表里被去掉的字符：%q", id)
		}
		if !IsShortID(id) {
			t.Fatalf("生成出来的短主键没通过自己的校验：%q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("第 %d 次出现重复短主键", i)
		}
		seen[id] = struct{}{}
	}
}

// 每一位都得跟着随机数变。少一次移位或掩码写错，某一位就会常年是同一个字符，
// 前一个测试看不出来——它只查字符集和长度。
func TestNewShortIDUsesEveryPosition(t *testing.T) {
	var distinct [8]map[byte]struct{}
	for i := range distinct {
		distinct[i] = make(map[byte]struct{})
	}
	for i := 0; i < 2000; i++ {
		id, err := NewShortID()
		if err != nil {
			t.Fatalf("生成短主键失败：%v", err)
		}
		for pos := 0; pos < 8; pos++ {
			distinct[pos][id[pos]] = struct{}{}
		}
	}
	for pos := 0; pos < 8; pos++ {
		if len(distinct[pos]) < 15 {
			t.Errorf("第 %d 位只出现过 %d 种字符，随机数没有铺满这一位", pos, len(distinct[pos]))
		}
	}
}

func TestIsShortID(t *testing.T) {
	accepted := []string{"a7k2m9pq", "00000000", "zzzzzzzz", "0a1b2c3d"}
	for _, s := range accepted {
		if !IsShortID(s) {
			t.Errorf("%q 应被接受", s)
		}
	}

	rejected := []struct {
		name  string
		input string
	}{
		{name: "空字符串", input: ""},
		{name: "少一位", input: "a7k2m9p"},
		{name: "多一位", input: "a7k2m9pqr"},
		{name: "大写", input: "A7K2M9PQ"},
		{name: "含 i", input: "a7k2m9pi"},
		{name: "含 l", input: "a7k2m9pl"},
		{name: "含 o", input: "a7k2m9po"},
		{name: "含 u", input: "a7k2m9pu"},
		{name: "含连字符", input: "a7k2m9-p"},
		{name: "含空格", input: "a7k2m9p "},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if IsShortID(tc.input) {
				t.Errorf("%q 应被拒绝", tc.input)
			}
		})
	}

	long, err := NewID()
	if err != nil {
		t.Fatalf("生成主键失败：%v", err)
	}
	if IsShortID(long) {
		t.Errorf("32 字符的主键不应被当成短主键：%q", long)
	}
}
