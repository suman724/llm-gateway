package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimitStore interface {
	// IncrementRPM increments the request counter for the tenant and returns the new value.
	IncrementRPM(ctx context.Context, tenantID string) (int64, error)
	// IncrementTPM increments the token counter and returns the new value.
	IncrementTPM(ctx context.Context, tenantID string, tokens int) (int64, error)
	// GetTPM returns the current token count for the tenant.
	GetTPM(ctx context.Context, tenantID string) (int64, error)
}

type RedisRateLimitStore struct {
	client *redis.Client
}

func NewRedisRateLimitStore(addr, password string) *RedisRateLimitStore {
	return &RedisRateLimitStore{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
		}),
	}
}

func (s *RedisRateLimitStore) IncrementRPM(ctx context.Context, tenantID string) (int64, error) {
	key := fmt.Sprintf("rate_limit:rpm:%s:%d", tenantID, time.Now().Unix()/60)
	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	if count == 1 {
		s.client.Expire(ctx, key, 90*time.Second) // Expire after 90s to be safe
	}
	return count, nil
}

func (s *RedisRateLimitStore) IncrementTPM(ctx context.Context, tenantID string, tokens int) (int64, error) {
	key := fmt.Sprintf("rate_limit:tpm:%s:%d", tenantID, time.Now().Unix()/60)
	count, err := s.client.IncrBy(ctx, key, int64(tokens)).Result()
	if err != nil {
		return 0, err
	}

	if count == int64(tokens) {
		s.client.Expire(ctx, key, 90*time.Second)
	}
	return count, nil
}

func (s *RedisRateLimitStore) GetTPM(ctx context.Context, tenantID string) (int64, error) {
	key := fmt.Sprintf("rate_limit:tpm:%s:%d", tenantID, time.Now().Unix()/60)
	val, err := s.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}
