package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

// 要比 nginx 的 proxy_read_timeout（15s）短。先由我们自己放弃，超时就是一条
// 能说清楚的错误，而不是一个被上游掐断的连接。
const putTimeout = 10 * time.Second

// 键里已经含内容摘要，内容变了键必然变，所以这份响应可以永久缓存。
const objectCacheControl = "public, max-age=31536000, immutable"

type OSSConfig struct {
	Region          string
	Bucket          string
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	PublicBaseURL   string
}

// 密钥不能跟着日志走，哪怕这个结构体只是被顺手打了一下。
func (c OSSConfig) String() string {
	return fmt.Sprintf(
		"{Region:%s Bucket:%s Endpoint:%s PublicBaseURL:%s AccessKeyID:<set> AccessKeySecret:<set>}",
		c.Region, c.Bucket, c.Endpoint, c.PublicBaseURL)
}

type OSSStore struct {
	client  *oss.Client
	bucket  string
	baseURL string
}

func NewOSSStore(config OSSConfig) *OSSStore {
	client := oss.NewClient(&oss.Config{
		Region:              oss.Ptr(config.Region),
		Endpoint:            oss.Ptr(config.Endpoint),
		CredentialsProvider: credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.AccessKeySecret),
	})
	return &OSSStore{
		client:  client,
		bucket:  config.Bucket,
		baseURL: strings.TrimSuffix(config.PublicBaseURL, "/"),
	}
}

func (s *OSSStore) Put(ctx context.Context, key string, data []byte, contentType string) (Object, error) {
	ctx, cancel := context.WithTimeout(ctx, putTimeout)
	defer cancel()

	// 签名版本不用挑：SDK 默认走 V4，而新建的 bucket 不认 V1。
	_, err := s.client.PutObject(ctx, &oss.PutObjectRequest{
		Bucket:       oss.Ptr(s.bucket),
		Key:          oss.Ptr(key),
		Body:         bytes.NewReader(data),
		ContentType:  oss.Ptr(contentType),
		CacheControl: oss.Ptr(objectCacheControl),
	})
	if err != nil {
		return Object{}, fmt.Errorf("上传 %s 到 OSS 失败: %w", key, err)
	}

	return Object{
		Key:         key,
		URL:         s.baseURL + "/" + key,
		Bytes:       int64(len(data)),
		ContentType: contentType,
	}, nil
}
