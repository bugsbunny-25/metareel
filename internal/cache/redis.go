package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bugsbunny-25/metareel/internal/config"
)

// ErrCacheMiss is returned when a key is not present in the cache.
var ErrCacheMiss = errors.New("cache: miss")

// Cache is a thin, typed wrapper around a Redis client that provides
// JSON (un)marshaling and a default TTL.
type Cache struct {
	client     *redis.Client
	defaultTTL time.Duration
}

// New creates a new Cache and verifies connectivity with a PING.
func New(ctx context.Context, cfg config.Redis) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Cache{client: client, defaultTTL: cfg.DefaultTTL}, nil
}

// Client exposes the underlying Redis client for callers that need more
// advanced functionality (pipelines, pub/sub, etc.).
func (c *Cache) Client() *redis.Client { return c.client }

// Close releases the underlying connection pool.
func (c *Cache) Close() error { return c.client.Close() }

// GetJSON reads a JSON-encoded value into dst. Returns ErrCacheMiss if the
// key does not exist.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) error {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return fmt.Errorf("cache get: %w", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("cache unmarshal: %w", err)
	}
	return nil
}

// SetJSON stores a JSON-encoded value. A ttl of 0 falls back to the
// configured default TTL.
func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

// Delete removes one or more keys.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}
