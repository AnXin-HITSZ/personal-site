package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/storage"
)

// 真的 1x1 PNG。用真数据而不是拼一个签名前缀，是因为嗅探要看的正是文件头，
// 拿假数据测出来的「通过」证明不了什么。
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func pngBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatalf("测试用的 PNG 解不开：%v", err)
	}
	return data
}

type recordingStore struct {
	keys []string
	data [][]byte
	err  error
}

func (s *recordingStore) Put(_ context.Context, key string, data []byte, contentType string) (storage.Object, error) {
	if s.err != nil {
		return storage.Object{}, s.err
	}
	s.keys = append(s.keys, key)
	s.data = append(s.data, data)
	return storage.Object{
		Key:         key,
		URL:         "https://cdn.example.com/" + key,
		Bytes:       int64(len(data)),
		ContentType: contentType,
	}, nil
}

type uploadFixture struct {
	store  *recordingStore
	router *gin.Engine
}

func newUploadFixture(store storage.Store) *uploadFixture {
	router := gin.New()
	router.Use(middleware.LogServerErrors(), middleware.Recovery())
	router.POST("/api/v1/admin/uploads", NewUploads(store).Create)

	recorded, _ := store.(*recordingStore)
	return &uploadFixture{store: recorded, router: router}
}

func (f *uploadFixture) upload(t *testing.T, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造表单失败：%v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("写入表单失败：%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭表单失败：%v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://anxin-hitsz.com/api/v1/admin/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestUploadAcceptsPNG(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	rec := f.upload(t, "photo.png", pngBytes(t))

	if rec.Code != http.StatusCreated {
		t.Fatalf("应 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if len(f.store.keys) != 1 {
		t.Fatalf("应上传一次，实际 %d 次", len(f.store.keys))
	}
	if !strings.HasPrefix(f.store.keys[0], "articles/") || !strings.HasSuffix(f.store.keys[0], ".png") {
		t.Errorf("对象键的格式不对：%q", f.store.keys[0])
	}
	if !bytes.Equal(f.store.data[0], pngBytes(t)) {
		t.Error("传上去的字节和原始文件不一致")
	}
	if !strings.Contains(rec.Body.String(), `"contentType":"image/png"`) {
		t.Errorf("响应应带上嗅探出来的类型：%s", rec.Body.String())
	}
}

// 内容寻址的直接好处：同一张图传两次是同一个键，OSS 上不会多出一个对象。
func TestUploadIsContentAddressed(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	first := f.upload(t, "a.png", pngBytes(t))
	second := f.upload(t, "b.png", pngBytes(t))

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("两次都该成功，实际 %d / %d", first.Code, second.Code)
	}
	if f.store.keys[0] != f.store.keys[1] {
		t.Errorf("同样的内容应得到同样的键，实际 %q 和 %q", f.store.keys[0], f.store.keys[1])
	}
}

func TestUploadRejectsDisguisedFile(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	// 扩展名说是图片，内容不是。以嗅探结果为准，所以过不去。
	rec := f.upload(t, "payload.png", []byte("<html><script>alert(1)</script></html>"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if len(f.store.keys) != 0 {
		t.Error("没通过校验就不该上传")
	}
}

// SVG 是 XML，能内嵌脚本，而 bucket 是公共读的。它被挡在外面是有意的，
// 所以要有测试钉住——不然哪天有人「顺手补上 svg 支持」是不会有人拦的。
func TestUploadRejectsSVG(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	rec := f.upload(t, "logo.svg", svg)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if len(f.store.keys) != 0 {
		t.Error("SVG 不该被上传")
	}
}

func TestUploadRejectsOversizedImage(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	// 前面是真的 PNG 头，所以它确实是图片——挡住它的是大小，不是类型。
	oversized := append(pngBytes(t), bytes.Repeat([]byte{0}, maxImageBytes)...)
	rec := f.upload(t, "huge.png", oversized)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("应 413，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if len(f.store.keys) != 0 {
		t.Error("超限的图片不该被上传")
	}
}

// 上面那条走的是「读完之后发现超了」；这一条走的是「读到一半就被 MaxBytesReader
// 掐断」。两条路径的错法完全不同，而后者认不出来的话，响应会变成一句
// 「请在 file 字段里放一张图片」——对着一个明明放了的文件说这种话。
func TestUploadRejectsOversizedBody(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	oversized := bytes.Repeat([]byte{0}, maxUploadBodyBytes+1)
	rec := f.upload(t, "huge.png", oversized)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("应 413，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCodeOf(t, rec); code != dto.CodePayloadTooLarge {
		t.Errorf("应为 %s，实际 %s", dto.CodePayloadTooLarge, code)
	}
}

func TestUploadWithoutFileField(t *testing.T) {
	f := newUploadFixture(&recordingStore{})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("caption", "没有文件")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "http://anxin-hitsz.com/api/v1/admin/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

// 没配 OSS 时不能回 200：那样作者拿到的是一个永远打不开的链接，而他不会知道
// 问题出在配置上。这里也不能并进 500——「存储没配」是个有明确下一步的原因。
func TestUploadWithoutStorageConfigured(t *testing.T) {
	f := newUploadFixture(storage.UnconfiguredStore{})

	rec := f.upload(t, "photo.png", pngBytes(t))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("应 503，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCodeOf(t, rec); code != dto.CodeServiceUnavailable {
		t.Errorf("应为 %s，实际 %s", dto.CodeServiceUnavailable, code)
	}
}

// 和上一条配成一对：配了存储但传失败，是这次请求坏了，不是这台机器没配好。
// 两条的码不一样，界面才能分开说话。
func TestUploadReportsStorageFailure(t *testing.T) {
	f := newUploadFixture(&recordingStore{err: errors.New("access denied")})

	rec := f.upload(t, "photo.png", pngBytes(t))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("应 500，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCodeOf(t, rec); code != dto.CodeInternalError {
		t.Errorf("应为 %s，实际 %s", dto.CodeInternalError, code)
	}
	if strings.Contains(rec.Body.String(), "access denied") {
		t.Error("存储层的错误细节不该出现在响应里")
	}
}
