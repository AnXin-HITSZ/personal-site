package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// 32 字节随机数。无 padding 的 base64url 保证它放进 URL 和 cookie 时不需要任何转义。
const tokenBytes = 32

func NewToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// 入库的是 sha256 的十六进制，不是令牌本身：数据库外流时攻击者拿到的东西
// 不能直接拿去冒充登录。校验靠唯一索引查这个值，因此不需要常量时间比较——
// 索引查找不泄漏秘密相关的信息。
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
