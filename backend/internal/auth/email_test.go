package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	accepted := []struct {
		name  string
		input string
		want  string
	}{
		{name: "原样通过", input: "owner@example.com", want: "owner@example.com"},
		{name: "去掉首尾空白并转小写", input: "  Owner@Example.COM  ", want: "owner@example.com"},
		{name: "保留加号标签和点", input: "first.last+site@mail.example.com", want: "first.last+site@mail.example.com"},
		{name: "子域名", input: "a@b.c.d.example.com", want: "a@b.c.d.example.com"},
		{name: "纯 IP 字面量域名", input: "a@[127.0.0.1]", want: "a@[127.0.0.1]"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeEmail(tc.input)
			if err != nil {
				t.Fatalf("应被接受，实际报错：%v", err)
			}
			if got != tc.want {
				t.Errorf("应为 %q，实际 %q", tc.want, got)
			}
		})
	}

	rejected := []struct {
		name  string
		input string
	}{
		{name: "空字符串", input: ""},
		{name: "只有空白", input: "   "},
		{name: "没有 @", input: "owner.example.com"},
		{name: "两个 @", input: "a@@example.com"},
		{name: "本地部分为空", input: "@example.com"},
		{name: "域名部分为空", input: "owner@"},
		{name: "中间有空格", input: "own er@example.com"},
		{name: "带显示名的形式", input: "Owner <owner@example.com>"},
		{name: "超过 255 字节", input: strings.Repeat("a", 250) + "@example.com"},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeEmail(tc.input); !errors.Is(err, ErrEmailInvalid) {
				t.Errorf("应返回 ErrEmailInvalid，实际 %v", err)
			}
		})
	}
}

// 展示名那条不是随手加的：`mail.ParseAddress` 会痛快地接受
// `Owner <owner@example.com>` 并返回 owner@example.com，只有拿结果和输入比对
// 才能发现调用者粘贴了一整行。这个测试就是钉住那次比对。
func TestNormalizeEmailRejectsParseableButDifferentInput(t *testing.T) {
	parsed, err := NormalizeEmail("owner@example.com")
	if err != nil || parsed != "owner@example.com" {
		t.Fatalf("构造前提不成立：%q %v", parsed, err)
	}
	if _, err := NormalizeEmail("owner@example.com <owner@example.com>"); !errors.Is(err, ErrEmailInvalid) {
		t.Error("解析结果与输入不一致时必须拒绝")
	}
}

// 归一化必须是幂等的：注册时算一次、登录时算一次，两次结果不同就等于两个账号。
func TestNormalizeEmailIsIdempotent(t *testing.T) {
	once, err := NormalizeEmail("  Mixed.Case@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := NormalizeEmail(once)
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("第二次归一化改变了结果：%q → %q", once, twice)
	}
}
