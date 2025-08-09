package cachestore

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrRateLimiterInternal = errors.New("internal error")
	ErrRateLimiterExceeded = errors.New("rate limit exceeded")
)

// Lua script for atomic token bucket operations
const script = `
	local key = KEYS[1]
	local capacity = tonumber(ARGV[1])
	local refill_rate = tonumber(ARGV[2])
	local refill_period = tonumber(ARGV[3])
	local now = tonumber(ARGV[4])

	-- Get current state
	local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
	local tokens = tonumber(bucket[1]) or capacity
	local last_refill = tonumber(bucket[2]) or now

	-- Calculate tokens to add
	local elapsed = now - last_refill
	local periods = math.floor(elapsed / refill_period)
	
	if periods > 0 then
		tokens = math.min(capacity, tokens + (periods * refill_rate))
		last_refill = last_refill + (periods * refill_period)
	end

	-- Try to consume one token
	local allowed = tokens > 0
	if allowed then
		tokens = tokens - 1
	end

	-- Update state
	redis.call('HMSET', key, 'tokens', tokens, 'last_refill', last_refill)
	redis.call('EXPIRE', key, refill_period * 2)

	return allowed and 1 or 0
`

// RateLimiterConfig holds the rate limiter configuration
type RateLimiterConfig struct {
	KeyPrefix    string        // Redis key prefix
	Capacity     int           // Maximum tokens in bucket
	RefillRate   int           // Tokens added per period
	RefillPeriod time.Duration // How often to refill tokens
}

// RateLimiter implements a Redis-based token bucket rate limiter
type RateLimiter struct {
	logger *slog.Logger
	client *redis.Client
	config RateLimiterConfig
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(logger *slog.Logger, cache *Cache, config RateLimiterConfig) RateLimiter {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "rate_limit:"
	}

	return RateLimiter{
		logger: logger,
		client: cache.rdb,
		config: config,
	}
}

// Allow checks if a request is allowed for the given key
func (rl RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	redisKey := rl.config.KeyPrefix + key
	now := time.Now().Unix()

	result, err := rl.client.Eval(ctx, script, []string{redisKey},
		rl.config.Capacity,
		rl.config.RefillRate,
		int(rl.config.RefillPeriod.Seconds()),
		now,
	).Result()

	if err != nil {
		rl.logger.Error("redis eval failed", "error", err)
		return false, ErrRateLimiterInternal
	}

	return result.(int64) == 1, nil
}

// UnaryServerInterceptor returns a gRPC interceptor that applies global rate limiting
func (rl RateLimiter) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		allowed, err := rl.Allow(ctx, "global")
		if err != nil {
			rl.logger.Error("rate limiter internal error", "error", err)
			return nil, status.Error(codes.Internal, ErrRateLimiterInternal.Error())
		}

		if !allowed {
			return nil, status.Error(codes.ResourceExhausted, ErrRateLimiterExceeded.Error())
		}

		return handler(ctx, req)
	}
}
