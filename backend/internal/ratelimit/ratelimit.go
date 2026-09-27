// 包 ratelimit 是进程内限速。它的边界要先说清楚：计数只活在这一个进程的内存里，
// 重启即清零，也管不到第二个副本。本站是单进程 systemd 服务，够用；
// 一旦起了第二个进程，限额会静默翻倍——没有任何报错，那才是必须换到 Redis 的信号。
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
	name    string
	windows map[string]*window
	limit   int
	period  time.Duration
	now     func() time.Time
	calls   int
}

// name 是这个限速器的身份，会被拼进最终的 key。
//
// 进程内这张表是每个 Limiter 自己的，加不加名字行为完全一样。之所以还是拼上，
// 是为了让这里和将来的共享存储实现在 key 的身份上一致：六个限速器里有三个
// 打在同一个 IP 上、三个打在同一个邮箱上，各存各的 map 时撞不上，
// 换到一个共用的 keyspace 就成了同一个计数器——不报错，只是限额算错。
// 名字由构造方给，调用方想漏也漏不掉。
func New(name string, limit int, period time.Duration) *Limiter {
	return &Limiter{
		name:    name,
		windows: make(map[string]*window),
		limit:   limit,
		period:  period,
		now:     time.Now,
	}
}

// 返回是否放行，以及被拦时还要等多久（用来填 Retry-After）。
// key 只给值，维度由 name 承担：传 "1.2.3.4" 或 "a@b.c"，不要自己拼前缀。
func (l *Limiter) Allow(rawKey string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls >= sweepInterval {
		l.calls = 0
		l.sweepLocked(l.now())
	}

	key := l.key(rawKey)
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
func (l *Limiter) Reset(rawKey string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, l.key(rawKey))
}

func (l *Limiter) key(raw string) string {
	return l.name + ":" + raw
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
