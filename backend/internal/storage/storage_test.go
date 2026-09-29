package storage

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestSniffImageAccepts(t *testing.T) {
	cases := []struct {
		name        string
		head        []byte
		contentType string
		extension   string
	}{
		{
			name:        "PNG",
			head:        append([]byte("\x89PNG\r\n\x1a\n"), []byte("....")...),
			contentType: "image/png",
			extension:   ".png",
		},
		{
			name:        "JPEG",
			head:        append([]byte("\xff\xd8\xff\xe0"), make([]byte, 8)...),
			contentType: "image/jpeg",
			extension:   ".jpg",
		},
		{
			name:        "GIF",
			head:        []byte("GIF89a\x01\x00\x01\x00"),
			contentType: "image/gif",
			extension:   ".gif",
		},
		{
			name:        "WebP",
			head:        []byte("RIFF\x24\x00\x00\x00WEBPVP8 "),
			contentType: "image/webp",
			extension:   ".webp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contentType, extension, ok := SniffImage(tc.head)
			if !ok {
				t.Fatalf("%s 应被接受", tc.name)
			}
			if contentType != tc.contentType || extension != tc.extension {
				t.Errorf("应为 %s/%s，实际 %s/%s", tc.contentType, tc.extension, contentType, extension)
			}
		})
	}
}

func TestSniffImageRejects(t *testing.T) {
	cases := []struct {
		name string
		head []byte
	}{
		{name: "空内容", head: nil},
		{name: "纯文本", head: []byte("这不是图片")},
		{name: "HTML", head: []byte("<html><body>hi</body></html>")},
		{name: "假装成 SVG 的脚本", head: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{name: "PDF", head: []byte("%PDF-1.7\n")},
		{name: "只有 PNG 的头两个字节", head: []byte("\x89P")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := SniffImage(tc.head); ok {
				t.Errorf("%s 不该被接受", tc.name)
			}
		})
	}
}

// SVG 单独钉一条：它是 XML，能内嵌脚本，而 bucket 是公共读的。
func TestSniffImageRejectsSVGExplicitly(t *testing.T) {
	// Go 的嗅探对 XML 声明给的是 text/xml，对裸 <svg> 给的是 text/plain，
	// 两种都不在放行名单里——但这条测试防的是「哪天有人把 image/svg+xml 加进去」。
	for _, head := range [][]byte{
		[]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
	} {
		contentType, _, ok := SniffImage(head)
		if ok {
			t.Errorf("SVG 不该被接受，嗅探结果是 %s", contentType)
		}
	}
}

func TestObjectKeyIsContentAddressed(t *testing.T) {
	first := sha256.Sum256([]byte("同一张图"))
	second := sha256.Sum256([]byte("同一张图"))
	other := sha256.Sum256([]byte("另一张图"))

	if ObjectKey(first, ".png") != ObjectKey(second, ".png") {
		t.Error("同样的内容应得到同样的键")
	}
	if ObjectKey(first, ".png") == ObjectKey(other, ".png") {
		t.Error("不同的内容应得到不同的键")
	}

	key := ObjectKey(first, ".png")
	if !strings.HasPrefix(key, "articles/") {
		t.Errorf("键应以 articles/ 开头：%q", key)
	}
	if !strings.HasSuffix(key, ".png") {
		t.Errorf("键应带上扩展名：%q", key)
	}
	// 摘要 64 个字符，前缀 2 个，两级之间一个斜杠，加 "articles/" 和扩展名。
	if want := len("articles/") + 2 + 1 + 64 + len(".png"); len(key) != want {
		t.Errorf("键长应为 %d，实际 %d：%q", want, len(key), key)
	}
	// 分两级是为了让单个前缀目录下的对象数不至于无限增长。
	if key[len("articles/")+2] != '/' {
		t.Errorf("前缀之后应再有一层目录：%q", key)
	}
}

func TestUnconfiguredStoreFails(t *testing.T) {
	if _, err := (UnconfiguredStore{}).Put(t.Context(), "articles/aa/x.png", []byte("x"), "image/png"); err == nil {
		t.Error("未配置时应报错，而不是安静地成功")
	}
}
