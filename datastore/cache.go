package datastore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// urlKeyPrefix is the label used for URL cache metrics.
	urlKeyPrefix = "url"
)

type Cache struct {
	rdb     *redis.Client
	metrics *CacheMetrics
}

func NewCache(ctx context.Context, connStr string, logger *slog.Logger) (Cache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: connStr,
	})

	// Set LFU eviction policy. This is best-effort. If it fails (e.g., permissions, old Redis version),
	// log a warning but continue. For this to have an effect, `maxmemory` must be set on the Redis server.
	// LFU is a great key eviction strategy for url shortening, because we want to always keep popular urls in the cache as much as possible.
	// So when we reach max memory, we evict least frequent accessed urls first.
	// To read more check https://redis.io/docs/latest/develop/reference/eviction.
	err := rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lfu").Err()
	if err != nil {
		logger.Warn("could not set redis maxmemory-policy to allkeys-lfu, ensure it is configured on the server", "error", err)
	}

	metrics, err := NewCacheMetrics()
	if err != nil {
		return Cache{}, fmt.Errorf("cache: failed to create metrics: %w", err)
	}

	c := Cache{
		rdb:     rdb,
		metrics: metrics,
	}

	if err := c.Ping(ctx); err != nil {
		return Cache{}, fmt.Errorf("cache: failed to ping redis: %w", err)
	}

	logger.Info("successfully connected to redis", "addr", connStr)
	return c, nil
}

func (c Cache) Ping(ctx context.Context) error {
	_, err := c.rdb.Ping(ctx).Result()
	return err
}

// Get retrieves a value from the cache. It returns redis.Nil if the key does not exist.
func (c Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, toInternalKey(key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			c.metrics.CacheMisses.WithLabelValues(urlKeyPrefix).Inc()
		}
		return "", err
	}
	c.metrics.CacheHits.WithLabelValues(urlKeyPrefix).Inc()
	return val, nil
}

// Set adds a key-value pair to the cache with an expiration.
func (c Cache) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	return c.rdb.Set(ctx, toInternalKey(key), value, expiration).Err()
}

func toInternalKey(s string) string {
	return fmt.Sprintf("%s:%s", urlKeyPrefix, s)
}

func (c Cache) Close() {
	_ = c.rdb.Close()
}
