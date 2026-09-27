package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// 16 字节随机数的十六进制，正好 32 个字符，与主键列的 VARCHAR(32) 等宽。
// 随机不是为了保密——主键会出现在 URL 里——而是为了让记录条数不可推测。
func NewID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
