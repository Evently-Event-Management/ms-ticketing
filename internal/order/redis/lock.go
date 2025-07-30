package redis

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type Redis struct {
	Client *redis.Client
}

func NewRedisClient(addr, password string, db int) *Redis {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &Redis{Client: rdb}
}

const lockTTL = 10 * time.Second // expire lock after 10 seconds

func (r *Redis) AcquireLock(key string) bool {
	ctx := context.Background()
	success, err := r.Client.SetNX(ctx, key, "locked", lockTTL).Result()
	if err != nil {
		return false
	}
	return success
}

func (r *Redis) ReleaseLock(key string) {
	ctx := context.Background()
	r.Client.Del(ctx, key)
}
