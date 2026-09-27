package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const testPassword = "correct horse battery staple"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	if hash == testPassword {
		t.Fatal("哈希与口令相同，等于没哈希")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("哈希不是 bcrypt 格式：%s", hash)
	}
	if !VerifyPassword(hash, testPassword) {
		t.Errorf("正确口令未通过校验")
	}
}

func TestHashPasswordIsSaltedPerCall(t *testing.T) {
	first, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	second, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	if first == second {
		t.Error("同一口令两次哈希相同，说明没有随机盐")
	}
	if !VerifyPassword(first, testPassword) || !VerifyPassword(second, testPassword) {
		t.Error("两个哈希都应能校验通过")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	cases := map[string]string{
		"空口令":    "",
		"差一个字符":  "correct horse battery stapl",
		"多一个字符":  testPassword + "e",
		"大小写不同":  strings.ToUpper(testPassword),
		"只差末尾空白": testPassword + " ",
	}
	for name, candidate := range cases {
		if VerifyPassword(hash, candidate) {
			t.Errorf("%s：不该通过校验", name)
		}
	}
}

func TestVerifyPasswordRejectsTamperedHash(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	tampered := []byte(hash)
	tampered[len(tampered)-1] ^= 0x01
	if VerifyPassword(string(tampered), testPassword) {
		t.Error("改过一个字符的哈希不该通过校验")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"x",
		"not-a-bcrypt-hash",
		"$2a$12$tooshort",
		"$9z$12$abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTU",
	}
	for _, hash := range cases {
		if VerifyPassword(hash, testPassword) {
			t.Errorf("格式非法的哈希不该通过校验：%q", hash)
		}
	}
}

// bcrypt 只接受 72 字节以内的输入，超过会返回错误。这里确认超长输入
// 既不会 panic，也不会意外通过。
func TestVerifyPasswordWithOverlongPassword(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	if VerifyPassword(hash, strings.Repeat("a", 200)) {
		t.Error("超长口令不该通过校验")
	}
	long, err := HashPassword(strings.Repeat("a", 200))
	if err == nil {
		if VerifyPassword(long, strings.Repeat("a", 200)) {
			t.Error("超长口令不该生成出可用的哈希")
		}
	} else if !errors.Is(err, ErrPasswordTooLong) && !strings.Contains(err.Error(), "72") {
		t.Errorf("超长口令的错误信息应说明长度限制，实际：%v", err)
	}
}

func TestValidatePasswordRejectsShort(t *testing.T) {
	for _, password := range []string{"", "a", strings.Repeat("a", MinPasswordRunes-1)} {
		if err := ValidatePassword(password, ""); !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("应报口令过短：%q，实际 %v", password, err)
		}
	}
	if err := ValidatePassword(strings.Repeat("a", MinPasswordRunes), ""); err != nil {
		t.Errorf("恰好达下限应通过，实际 %v", err)
	}
}

func TestValidatePasswordCountsRunesNotBytes(t *testing.T) {
	// 12 个汉字是 36 字节：按字符计量应通过，按字节计量会被误判。
	password := strings.Repeat("密", MinPasswordRunes)
	if err := ValidatePassword(password, ""); err != nil {
		t.Errorf("12 个汉字应通过，实际 %v", err)
	}
}

func TestValidatePasswordRejectsOverBCryptLimit(t *testing.T) {
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes), ""); err != nil {
		t.Errorf("恰好 %d 字节应通过，实际 %v", MaxPasswordBytes, err)
	}
	err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes+1), "")
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("超出 %d 字节应报过长，实际 %v", MaxPasswordBytes, err)
	}
	// 25 个汉字是 75 字节，超过 bcrypt 的限制——若只按字符数判断会漏掉。
	if err := ValidatePassword(strings.Repeat("密", MaxPasswordBytes/3+1), ""); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("超长汉字口令应报过长，实际 %v", err)
	}
}

func TestValidatePasswordRejectsSameAsEmail(t *testing.T) {
	email := "anxin@example.com"
	if err := ValidatePassword(email, email); !errors.Is(err, ErrPasswordSameMail) {
		t.Errorf("口令与邮箱相同应被拒绝，实际 %v", err)
	}
	if err := ValidatePassword(strings.ToUpper(email), email); !errors.Is(err, ErrPasswordSameMail) {
		t.Errorf("大小写不同也算相同，实际 %v", err)
	}
	if err := ValidatePassword(testPassword, email); err != nil {
		t.Errorf("普通口令应通过，实际 %v", err)
	}
	if err := ValidatePassword(testPassword, ""); err != nil {
		t.Errorf("没有邮箱时不该比较，实际 %v", err)
	}
}

// 字母表里混进一个重复字符不会报错，只会让分布偏一点——所以逐个数字符，
// 而不是只数个数。
func TestPasswordAlphabetHasNoDuplicatesOrLookalikes(t *testing.T) {
	if len(passwordAlphabet) != 55 {
		t.Errorf("字母表应有 55 个字符，实际 %d", len(passwordAlphabet))
	}
	seen := map[rune]bool{}
	for _, char := range passwordAlphabet {
		if seen[char] {
			t.Errorf("字符重复：%q", char)
		}
		seen[char] = true
		if strings.ContainsRune("ilIoO01", char) {
			t.Errorf("易混淆字符不该出现在口令里：%q", char)
		}
	}
}

func TestNewPasswordShape(t *testing.T) {
	password, err := NewPassword()
	if err != nil {
		t.Fatalf("生成口令失败：%v", err)
	}
	if len([]rune(password)) != GeneratedPasswordRunes {
		t.Errorf("应为 %d 个字符，实际 %d：%q", GeneratedPasswordRunes, len([]rune(password)), password)
	}
	for _, char := range password {
		if !strings.ContainsRune(passwordAlphabet, char) {
			t.Fatalf("出现字母表以外的字符 %q：%q", char, password)
		}
	}
	// 生成的初始口令要被自己的校验规则接受，否则运维抄下来登录时才发现白跑一趟。
	if err := ValidatePassword(password, ""); err != nil {
		t.Errorf("初始口令应满足口令规则，实际 %v", err)
	}
}

func TestNewPasswordIsNotRepeated(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		password, err := NewPassword()
		if err != nil {
			t.Fatalf("生成口令失败：%v", err)
		}
		if seen[password] {
			t.Fatalf("重复生成了同一个口令：%q", password)
		}
		seen[password] = true
	}
}

func TestNewPasswordIsVerifiableAsHash(t *testing.T) {
	password, err := NewPassword()
	if err != nil {
		t.Fatalf("生成口令失败：%v", err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("生成哈希失败：%v", err)
	}
	if !VerifyPassword(hash, password) {
		t.Error("初始口令的哈希应能校验通过")
	}
}

// 替身哈希只在代价与被模仿的比较相同时才有意义——代价低了，快慢差原样回来，
// 而且不会有任何报错。bcryptCost 改动的当天这条就会红。
func TestDummyPasswordHashCostMatches(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(DummyPasswordHash))
	if err != nil {
		t.Fatalf("替身哈希不是合法的 bcrypt：%v", err)
	}
	if cost != bcryptCost {
		t.Errorf("替身哈希代价应为 %d，实际 %d", bcryptCost, cost)
	}
}
