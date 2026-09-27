package auth

import (
	"errors"
	"net/mail"
	"strings"
)

const MaxEmailBytes = 255

var ErrEmailInvalid = errors.New("邮箱格式不合法")

// 归一化和校验放在一起，是为了没有「只归一化不校验」或反过来的调用点。
// 统一转小写：列的排序规则虽然也能让大小写不敏感地比较，但库里存的值就不可
// 预测了，而 Go 里比较两个邮箱时排序规则是帮不上忙的。
func NormalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || len(normalized) > MaxEmailBytes {
		return "", ErrEmailInvalid
	}
	// 要求解析结果和输入完全一致，是为了挡掉 `张三 <a@b.com>` 这种粘贴进来的形式。
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized {
		return "", ErrEmailInvalid
	}
	return normalized, nil
}
