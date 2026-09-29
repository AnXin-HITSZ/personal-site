// 包 storage 只管把字节放进对象存储。它不认识文章、作者或任何业务概念——
// 键名和 MIME 类型由调用方给。
//
// 三个实现，由 main.go 按配置挑：OSSStore 真的上传；LogStore 只把键名和大小
// 打进日志，给开发和测试用；UnconfiguredStore 一律失败——没配 OSS 时让上传
// 接口明确地回 503，好过安静地成功，然后给出一串打不开的图片链接。
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
)

var ErrNotConfigured = errors.New("对象存储未配置")

type Object struct {
	Key         string
	URL         string
	Bytes       int64
	ContentType string
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (Object, error)
}

type LogStore struct{}

func (LogStore) Put(_ context.Context, key string, data []byte, contentType string) (Object, error) {
	log.Printf("[storage:dev] 键=%s 类型=%s 字节=%d（没有真的上传，这个链接打不开）", key, contentType, len(data))
	return Object{
		Key:         key,
		URL:         "https://storage.invalid/" + key,
		Bytes:       int64(len(data)),
		ContentType: contentType,
	}, nil
}

type UnconfiguredStore struct{}

func (UnconfiguredStore) Put(context.Context, string, []byte, string) (Object, error) {
	return Object{}, ErrNotConfigured
}

// SVG 不在里面，这是有意的：它是 XML，可以内嵌脚本，而 bucket 是公共读的
// ——放进去等于给自己开一个 XSS 托管。
var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// DetectContentType 只看前 512 字节，调用方给这么多就够，不必把整张图读进内存。
// 扩展名以嗅探结果为准，不信文件名——文件名是用户说了算的。
func SniffImage(head []byte) (contentType, extension string, ok bool) {
	contentType = http.DetectContentType(head)
	extension, ok = imageExtensions[contentType]
	return contentType, extension, ok
}

// 按内容定址：同一张图传两次是同一个键，天然去重；键变了内容必然跟着变，
// 所以那份响应可以永久缓存。
func ObjectKey(sum [sha256.Size]byte, extension string) string {
	digest := hex.EncodeToString(sum[:])
	return "articles/" + digest[:2] + "/" + digest + extension
}
