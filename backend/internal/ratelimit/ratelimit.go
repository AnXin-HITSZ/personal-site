// 包 ratelimit 是进程内限速。它的边界要先说清楚：计数只活在这一个进程的内存里，
// 重启即清零，也管不到第二个副本。本站是单进程 systemd 服务，够用；
// 哪天要跑多副本或者要更严的保证，就得换到 Redis 上。
//
// 用固定窗口，不用令牌桶：窗口从某个 key 的第一次请求起算，实现只有几十行，
// 出错的地方肉眼可见。代价是窗口边界处最多能过 2×limit 次——对这个站点无所谓。
package ratelimit

import (
	"sync"
	"time"
)

// 每扫这么多次做一次全表清理。没有后台 goroutine：只在这里顺手清理，
// 就没有需要管生命周期的东西，进程退出时也不会有协程泄漏。
const sweepInterval = 1024

// 表的上限。超过就先清理；清理完还超，整张表丢掉重来——宁可短暂失去限速，
// 也不要让内存被撑爆，把整个服务一起拖死。
const maxKeys = 100_000

type window struct {
	count   int
	expires time.Time
}

type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	limit   int
	period  time.Duration
	now     func() time.Time
	calls   int
}

func New(limit int, period time.Duration) *Limiter {
	return &Limiter{
		windows: make(map[string]*window),
		limit:   limit,
		period:  period,
		now:     time.Now,
	}
}

// 返回是否放行，以及被拦时还要等多久（用来填 Retry-After）。
// key 由调用方拼：同一个 Limiter 上要区分维度时用前缀，例如 "ip:1.2.3.4"、"mail:a@b.c"。
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls >= sweepInterval {
		l.calls = 0
		l.sweepLocked(l.now())
	}

	now := l.now()
	current, ok := l.windows[key]
	if !ok || !now.Before(current.expires) {
		l.windows[key] = &window{count: 1, expires: now.Add(l.period)}
		return true, 0
	}

	current.count++
	if current.count > l.limit {
		return false, current.expires.Sub(now)
	}
	return true, 0
}

// 登录成功后清掉这个 key。只清「按邮箱」那一维，绝不清「按 IP」那一维——
// 否则攻击者只要手里有一个能登录的账号，就能靠它不断把自己的 IP 计数清零，
// 拿同一个 IP 继续爆破别人。
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, key)
}

func (l *Limiter) sweepLocked(now time.Time) {
	for key, current := range l.windows {
		if !now.Before(current.expires) {
			delete(l.windows, key)
		}
	}
	if len(l.windows) > maxKeys {
		l.windows = make(map[string]*window)
	}
}
