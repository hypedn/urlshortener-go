package cachestore

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

	// cacheConnectTimeout is the timeout for establishing redis connection.
	cacheConnectTimeout = 15 * time.Second

	// urlTTL is the time to expire keys on the cache.
	urlTTL = time.Hour
)

type Cache struct {
	rdb     *redis.Client
	metrics Metrics
	logger  *slog.Logger
}

func NewCache(ctx context.Context, connStr string, logger *slog.Logger) (*Cache, error) {
	ctx, cancel := context.WithTimeout(ctx, cacheConnectTimeout)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr: connStr,
	})

	c := &Cache{
		rdb:     rdb,
		logger:  logger,
		metrics: NewMetrics(),
	}

	if err := c.Ping(ctx); err != nil {
		return &Cache{}, fmt.Errorf("cache: failed to ping redis: %w", err)
	}

	// Set LFU eviction policy. This is best-effort. If it fails (e.g., permissions, old Redis version),
	// log a warning but continue. For this to have an effect, `maxmemory` must be set on the Redis server.
	// LFU is a great key eviction strategy for url shortening, because we want to always keep popular urls in the cache as much as possible.
	// So when we reach max memory, we evict least frequent accessed urls first.
	// To read more check https://redis.io/docs/latest/develop/reference/eviction.
	err := rdb.ConfigSet(ctx, "maxmemory-policy", "allkeys-lfu").Err()
	if err != nil {
		logger.Warn("could not set redis maxmemory-policy to allkeys-lfu, ensure it is configured on the server", "error", err)
	}

	return c, nil
}

func (c Cache) Ping(ctx context.Context) error {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Loop until the context is cancelled or the ping is successful.
	for {
		_, err := c.rdb.Ping(ctx).Result()
		if err == nil {
			break // Ping successful.
		}

		c.logger.Warn("unable to establish connection, retrying...", "error", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("db connection timed out or was cancelled: %w (last error: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
	}
	return nil
}

// GetURL retrieves an URL from the cache. It returns redis.Nil if the key does not exist.
func (c Cache) GetURL(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, toInternalKey(key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			c.metrics.Misses.WithLabelValues(urlKeyPrefix).Inc()
		}
		return "", err
	}
	c.metrics.Hits.WithLabelValues(urlKeyPrefix).Inc()
	return val, nil
}

// SetURL adds a key-value pair to the cache.
func (c Cache) SetURL(ctx context.Context, key string, value string) error {
	return c.rdb.Set(ctx, toInternalKey(key), value, urlTTL).Err()
}

func toInternalKey(s string) string {
	return fmt.Sprintf("%s:%s", urlKeyPrefix, s)
}

func (c Cache) Close() {
	_ = c.rdb.Close()
}
