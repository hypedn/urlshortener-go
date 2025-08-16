package config

import (
	"time"

	"github.com/hypedn/mflag"
)

const (
	appGrpcEndpoint = "grpc_endpoint"
	appHttpEndpoint = "http_endpoint"
	appDBAddress    = "db_address"
)

const (
	redisKey       = "redis"
	redisAddr      = "address"
	redisPoolSize  = "pool_size"
	redisUrlTTL    = "url_ttl"
	redisUrlPrefix = "url_prefix"
)

const (
	rateLimiterKey          = "rate_limiter"
	rateLimiterKeyPrefix    = "key_prefix"
	rateLimiterCapacity     = "capacity"
	rateLimiterRefillRate   = "refill_rate"
	rateLimiterRefillPeriod = "refill_period"
)

type AppSettings struct {
	GrpcEndpoint string
	HttpEndpoint string
	DBAddress    string
}

type Redis struct {
	Addr      string
	UrlPrefix string
	PoolSize  int
	UrlTTL    time.Duration
}

type RateLimiter struct {
	KeyPrefix    string        // Redis key prefix
	Capacity     int           // Maximum tokens in bucket
	RefillRate   int           // Tokens added per period
	RefillPeriod time.Duration // How often to refill tokens
}

func SetDefaults() {
	mflag.SetDefault(appHttpEndpoint, "localhost:8080")
	mflag.SetDefault(appGrpcEndpoint, "localhost:8081")
	mflag.SetDefault(appDBAddress, "postgres://ndev:@localhost:5432/urlshortener?sslmode=disable")

	mflag.SetDefault(redisKey, map[string]interface{}{
		redisAddr:      "localhost:6379",
		redisPoolSize:  10,
		redisUrlTTL:    time.Hour,
		redisUrlPrefix: "url",
	})
	mflag.SetDefault(rateLimiterKey, map[string]interface{}{
		rateLimiterKeyPrefix:    "ratelimit:", // global rate limiter key
		rateLimiterCapacity:     10,           // 10 token burst
		rateLimiterRefillRate:   40,           // 40 tokens per period
		rateLimiterRefillPeriod: time.Second,  // Every second
	})
}

func GetSettings() (
	AppSettings,
	Redis,
	RateLimiter,
) {
	return AppSettings{
			GrpcEndpoint: mflag.GetString(appGrpcEndpoint),
			HttpEndpoint: mflag.GetString(appHttpEndpoint),
			DBAddress:    mflag.GetString(appDBAddress),
		},
		Redis{
			Addr:      mflag.GetString(redisAddr),
			PoolSize:  mflag.GetInt(redisPoolSize),
			UrlTTL:    mflag.GetDuration(redisUrlTTL),
			UrlPrefix: mflag.GetString(redisUrlPrefix),
		},
		RateLimiter{
			KeyPrefix:    mflag.GetString(rateLimiterKeyPrefix),
			Capacity:     mflag.GetInt(rateLimiterCapacity),
			RefillRate:   mflag.GetInt(rateLimiterRefillRate),
			RefillPeriod: mflag.GetDuration(rateLimiterRefillPeriod),
		}
}
