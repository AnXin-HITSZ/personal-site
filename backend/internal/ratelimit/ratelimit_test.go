package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// 时钟注入进来，测试才能一秒钟跨过一小时的窗口。
// 名字固定成 test：这里的用例都在验窗口的行为，名字参不参与 key 由下面单独测。
func newTestLimiter(limit int, period time.Duration) (*Limiter, func(time.Duration)) {
	limiter := New("test", limit, period)
	current := testNow
	limiter.now = func() time.Time { return current }
	return limiter, func(d time.Duration) { current = current.Add(d) }
}

func TestAllowsUpToLimitThenBlocks(t *testing.T) {
	limiter, _ := newTestLimiter(3, time.Hour)

	for attempt := 1; attempt <= 3; attempt++ {
		allowed, retryAfter := limiter.Allow("ip:1.2.3.4")
		if !allowed {
			t.Fatalf("第 %d 次应放行", attempt)
		}
		if retryAfter != 0 {
			t.Errorf("放行时不该给等待时间，实际 %v", retryAfter)
		}
	}

	allowed, retryAfter := limiter.Allow("ip:1.2.3.4")
	if allowed {
		t.Fatal("第 4 次应被拦下")
	}
	if retryAfter != time.Hour {
		t.Errorf("Retry-After 应为整整一个窗口，实际 %v", retryAfter)
	}
}

func TestWindowResetsAfterPeriod(t *testing.T) {
	limiter, advance := newTestLimiter(2, 15*time.Minute)
	limiter.Allow("ip:1.2.3.4")
	limiter.Allow("ip:1.2.3.4")
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); allowed {
		t.Fatal("构造前提不成立：应已被拦")
	}

	advance(15 * time.Minute)
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); !allowed {
		t.Error("窗口过后应重新放行")
	}
}

// 边界条件：正好到期的这一刻就该重新放行，不能多等一个纳秒。
func TestWindowExpiresExactlyAtBoundary(t *testing.T) {
	limiter, advance := newTestLimiter(1, time.Minute)
	limiter.Allow("ip:1.2.3.4")

	advance(time.Minute - time.Nanosecond)
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); allowed {
		t.Error("还差一纳秒就不该放行")
	}
	advance(time.Nanosecond)
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); !allowed {
		t.Error("正好到期就应放行")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	limiter, _ := newTestLimiter(1, time.Hour)

	if allowed, _ := limiter.Allow("ip:1.2.3.4"); !allowed {
		t.Fatal("构造前提不成立")
	}
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); allowed {
		t.Fatal("同一个 key 应被拦")
	}
	// 一个 IP 被限住，不该牵连另一个 IP 或另一维度的 key。
	for _, key := range []string{"ip:5.6.7.8", "mail:a@b.c", "ip:1.2.3.40"} {
		if allowed, _ := limiter.Allow(key); !allowed {
			t.Errorf("%s 是独立的 key，不该被牵连", key)
		}
	}
}

func TestResetClearsOneKeyOnly(t *testing.T) {
	limiter, _ := newTestLimiter(1, time.Hour)
	limiter.Allow("mail:a@b.c")
	limiter.Allow("ip:1.2.3.4")

	limiter.Reset("mail:a@b.c")

	if allowed, _ := limiter.Allow("mail:a@b.c"); !allowed {
		t.Error("被重置的 key 应重新放行")
	}
	if allowed, _ := limiter.Allow("ip:1.2.3.4"); allowed {
		t.Error("重置一个 key 不该动到别的 key")
	}
}

func TestResetUnknownKeyIsHarmless(t *testing.T) {
	limiter, _ := newTestLimiter(1, time.Hour)
	limiter.Reset("从来没有过的 key")

	if allowed, _ := limiter.Allow("ip:1.2.3.4"); !allowed {
		t.Error("重置不存在的 key 不该影响后续")
	}
}

// 清理是顺手做的，所以必须确认它真的会发生——否则过期条目会一直堆着，
// 表只增不减。
func TestExpiredEntriesAreSweptAway(t *testing.T) {
	limiter, advance := newTestLimiter(5, time.Minute)
	for index := range 200 {
		limiter.Allow(fmt.Sprintf("key-%d", index))
	}
	if len(limiter.windows) != 200 {
		t.Fatalf("构造前提不成立：应有 200 个 key，实际 %d", len(limiter.windows))
	}

	advance(2 * time.Minute)
	for range sweepInterval {
		limiter.Allow("trigger")
	}

	if len(limiter.windows) > 2 {
		t.Errorf("过期条目应被清掉，实际还剩 %d 个", len(limiter.windows))
	}
}

// 表超过上限时整张丢掉：宁可短暂不限速，也不能让内存无限涨。
func TestOversizedTableIsDropped(t *testing.T) {
	limiter, _ := newTestLimiter(1, time.Hour)
	// 没有一个是过期的：这样清理只会删掉零个，才试得出「超过上限就整张丢掉」。
	for index := range maxKeys + 10 {
		limiter.windows[fmt.Sprintf("key-%d", index)] = &window{count: 1, expires: testNow.Add(time.Hour)}
	}

	limiter.sweepLocked(testNow)

	if len(limiter.windows) != 0 {
		t.Errorf("超过上限后应清空，实际还剩 %d 个", len(limiter.windows))
	}
}

// 名字是 key 身份的一部分。同一个值落在两个限速器上必须是两个计数器——
// 这是给将来的共享存储实现立的规矩：六个限速器里有三个用同一个 IP、
// 三个用同一个邮箱，各存各的 map 时撞不上，共用一张表时全靠这个名字分开。
func TestNameSeparatesLimitersSharingAKey(t *testing.T) {
	login := New("login:ip", 1, time.Hour)
	register := New("register:ip", 1, time.Hour)

	if login.key("1.2.3.4") == register.key("1.2.3.4") {
		t.Fatalf("两个限速器的 key 不该相同，都是 %q", login.key("1.2.3.4"))
	}

	if allowed, _ := login.Allow("1.2.3.4"); !allowed {
		t.Fatal("构造前提不成立")
	}
	if allowed, _ := login.Allow("1.2.3.4"); allowed {
		t.Fatal("同一个限速器的第二次应被拦")
	}
	if allowed, _ := register.Allow("1.2.3.4"); !allowed {
		t.Error("登录维度被限住，不该牵连注册维度")
	}
}

func TestConcurrentAccessIsSerialised(t *testing.T) {
	limiter := New("test", 50, time.Hour)

	var wait sync.WaitGroup
	allowed := make([]int, 100)
	for worker := range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 20 {
				if ok, _ := limiter.Allow("ip:1.2.3.4"); ok {
					allowed[worker]++
				}
			}
		}()
	}
	wait.Wait()

	total := 0
	for _, count := range allowed {
		total += count
	}
	// 固定窗口下，只有前 50 次能过。加锁漏了的话这里会明显偏大。
	if total != 50 {
		t.Errorf("并发下应恰好放行 50 次，实际 %d", total)
	}
}
