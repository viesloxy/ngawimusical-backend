package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/viesloxy/ngawimusical-backend/config"
)

// NewAdmissionRedis membuat koneksi Redis untuk admission control dan rate limit.
func NewAdmissionRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	return newRedis(ctx, cfg.AdmissionAddr, cfg.AdmissionPassword, cfg.AdmissionDB, "admission")
}

// NewQueueRedis membuat koneksi Redis untuk antrean pekerjaan Asynq.
func NewQueueRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	return newRedis(ctx, cfg.QueueAddr, cfg.QueuePassword, cfg.QueueDB, "queue")
}

func newRedis(ctx context.Context, addr, password string, db int, name string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis %s: %w", name, err)
	}

	return client, nil
}
