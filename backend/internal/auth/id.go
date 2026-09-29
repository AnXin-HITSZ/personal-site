package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
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

// Crockford base32 的小写字母表，少了 i、l、o、u：手抄或口述地址时，
// 这几个字符最容易和 1、1、0、v 混起来。
const shortIDAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// 5 字节随机数正好编成 8 个字符——40 位按每字符 5 位排满，不多一个填充位。
// 它比 NewID 短得多，因为它要出现在文章地址里；代价是只有 40 位随机，
// 碰撞交给主键唯一约束兜底，调用方重试一次即可。
func NewShortID() (string, error) {
	raw := make([]byte, 5)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	n := uint64(raw[0])<<32 | uint64(raw[1])<<24 | uint64(raw[2])<<16 | uint64(raw[3])<<8 | uint64(raw[4])
	var out [8]byte
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = shortIDAlphabet[n&0x1f]
		n >>= 5
	}
	return string(out[:]), nil
}

// IsShortID 判断一个字符串是不是 NewShortID 可能产出的形状。
// 用途是让 handler 在打数据库之前就把形状不对的地址挡掉。
func IsShortID(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(shortIDAlphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}
