package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const defaultTTL = 5 * time.Minute

// RedisCache wraps Redis for JSON caching.
type RedisCache struct {
	rdb    *redis.Client
	ttl    time.Duration
	logger *zap.Logger
}

// NewRedisCache creates a cache backed by Redis.
func NewRedisCache(rdb *redis.Client, ttl time.Duration, logger *zap.Logger) *RedisCache {
	if ttl == 0 {
		ttl = defaultTTL
	}
	return &RedisCache{rdb: rdb, ttl: ttl, logger: logger}
}

// Get retrieves a cached value by key, returns false if miss.
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) bool {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		c.logger.Warn("cache get error", zap.String("key", key), zap.Error(err))
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		c.logger.Warn("cache unmarshal error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// Set stores a value in cache with TTL.
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		c.logger.Warn("cache marshal error", zap.String("key", key), zap.Error(err))
		return
	}
	if err := c.rdb.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.logger.Warn("cache set error", zap.String("key", key), zap.Error(err))
	}
}

// Delete removes a key from cache.
func (c *RedisCache) Delete(ctx context.Context, key string) {
	c.rdb.Del(ctx, key)
}

// SearchKey generates a cache key for a search request.
func SearchKey(query, mode, source, lang string, page, pageSize int) string {
	return fmt.Sprintf("search:%s:%s:%s:%s:%d:%d", query, mode, source, lang, page, pageSize)
}
