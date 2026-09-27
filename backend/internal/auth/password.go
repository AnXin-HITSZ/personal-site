package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// 代价 12 约合 250ms：登录时付一次可以接受，爆破则每次都要付同样的代价。
const bcryptCost = 12

// 下限按字符数——用户感知的是「几个字」；上限按字节数——bcrypt 的硬限制是 72 字节，
// 超出会报错。12 个汉字是 36 字节，仍在限制内。
const (
	MinPasswordRunes = 12
	MaxPasswordBytes = 72
)

var (
	ErrPasswordTooShort = errors.New("口令过短")
	ErrPasswordTooLong  = errors.New("口令过长")
	ErrPasswordSameMail = errors.New("口令不能与邮箱相同")
)

func ValidatePassword(password, email string) error {
	if utf8.RuneCountInString(password) < MinPasswordRunes {
		return fmt.Errorf("%w：至少 %d 个字符", ErrPasswordTooShort, MinPasswordRunes)
	}
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w：最多 %d 字节（约 %d 个 ASCII 字符，或 %d 个汉字）",
			ErrPasswordTooLong, MaxPasswordBytes, MaxPasswordBytes, MaxPasswordBytes/3)
	}
	if email != "" && strings.EqualFold(password, strings.TrimSpace(email)) {
		return ErrPasswordSameMail
	}
	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// 去掉了 i/l/o 和 I/O 以及 0/1：字母和数字长得太像，手抄一次就会抄错。
const (
	GeneratedPasswordRunes = 24
	passwordAlphabet       = "abcdefghjkmnpqrstuvwxyz" + "ABCDEFGHJKLMNPQRSTUVWXYZ" + "23456789"
)

// 初始口令。用 rand.Int 而不是 rand.Read 取模：55 个字符除不尽 256，
// 取模会让前 28 个字符出现的概率高出一截。24 个字符约 139 比特，够用很久。
func NewPassword() (string, error) {
	limit := big.NewInt(int64(len(passwordAlphabet)))
	var builder strings.Builder
	builder.Grow(GeneratedPasswordRunes)
	for range GeneratedPasswordRunes {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(passwordAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

// 只回答「是否匹配」。bcrypt 的错误分好几种（哈希格式不对、代价越界、口令不匹配），
// 但调用方需要知道的只有这一位，把细节漏出去只会多一个探测面。
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// 口令为 "never-matches-anything" 的 bcrypt 哈希。它不是任何账号的口令，
// 唯一的用途是：登录时邮箱查不到，也拿它走一遍同样代价的比较。
// 少了这一步，「邮箱不存在」会立刻返回，「口令错误」要等 250ms——响应快慢
// 就把哪些邮箱注册过说出来了。比较结果直接丢掉。
//
// 代价必须和 bcryptCost 一致，否则快慢差又回来了（见 DummyPasswordHashCostMatches）。
const DummyPasswordHash = "$2a$12$K46.OILEPconGYoYpYjPx.yFqOkU34W.YgPyWCvlxrfsjDCORC5.K"
