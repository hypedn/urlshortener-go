package datastore

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// KeyPrefixLabel is the label for cache metrics, representing the key prefix.
	KeyPrefixLabel = "key_prefix"
)

// Metrics contains the Prometheus collectors for cache-related metrics.
type CacheMetrics struct {
	CacheHits   *prometheus.CounterVec
	CacheMisses *prometheus.CounterVec
	CacheSize   *prometheus.GaugeVec
}

// NewCacheMetrics creates and registers the cache metrics collectors.
// It returns an error if any of the collectors fail to register.
func NewCacheMetrics() (*CacheMetrics, error) {
	m := &CacheMetrics{
		CacheHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_hit_count",
			Help: "The number of cache hits",
		}, []string{KeyPrefixLabel}),
		CacheMisses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_miss_count",
			Help: "The number of cache misses",
		}, []string{KeyPrefixLabel}),
		CacheSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "The size of a set within the cache, identified by its key",
		}, []string{KeyPrefixLabel}),
	}

	collectors := []prometheus.Collector{
		m.CacheHits,
		m.CacheMisses,
		m.CacheSize,
	}
	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			return nil, err
		}
	}
	return m, nil
}
