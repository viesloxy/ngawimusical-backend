package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter membatasi jumlah percobaan per kunci dalam satu jendela waktu.
// Allow mengembalikan false beserta sisa waktu tunggu bila batas sudah terlampaui.
type Limiter interface {
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
}

// MemoryLimiter adalah limiter fixed-window per proses.
// ponytail: state hanya ada di satu proses; gunakan RedisLimiter bila API dijalankan lebih dari satu instance.
type MemoryLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	hits      map[string]windowCount
	lastSweep time.Time
}

type windowCount struct {
	start time.Time
	count int
}

func NewMemoryLimiter(limit int, window time.Duration) *MemoryLimiter {
	return &MemoryLimiter{limit: limit, window: window, hits: map[string]windowCount{}}
}

func (l *MemoryLimiter) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Buang jendela yang sudah lewat sekali per window agar memori tidak tumbuh tanpa batas.
	if now.Sub(l.lastSweep) >= l.window {
		for k, v := range l.hits {
			if now.Sub(v.start) >= l.window {
				delete(l.hits, k)
			}
		}
		l.lastSweep = now
	}

	w := l.hits[key]
	if now.Sub(w.start) >= l.window {
		w = windowCount{start: now}
	}
	if w.count >= l.limit {
		return false, w.start.Add(l.window).Sub(now), nil
	}
	w.count++
	l.hits[key] = w
	return true, 0, nil
}

// RedisLimiter menghitung percobaan di Redis agar berlaku lintas instance.
// Jika Redis tidak tersedia, limiter memakai fallback memori.
type RedisLimiter struct {
	client   *redis.Client
	limit    int64
	window   time.Duration
	fallback *MemoryLimiter
}

func NewRedisLimiter(client *redis.Client, limit int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{
		client:   client,
		limit:    int64(limit),
		window:   window,
		fallback: NewMemoryLimiter(limit, window),
	}
}

// incrWithTTL menaikkan hitungan dan memasang expiry dalam satu eksekusi atomik.
// Bila key tidak punya expiry (TTL -1), expiry dipasang ulang agar tidak terkunci permanen.
var incrWithTTL = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
	ttl = tonumber(ARGV[1])
end
return {n, ttl}`)

func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	redisKey := "ratelimit:" + key

	res, err := incrWithTTL.Run(ctx, l.client, []string{redisKey}, l.window.Milliseconds()).Int64Slice()
	if err != nil || len(res) != 2 {
		return l.fallback.Allow(ctx, key)
	}
	if res[0] <= l.limit {
		return true, 0, nil
	}
	return false, time.Duration(res[1]) * time.Millisecond, nil
}
