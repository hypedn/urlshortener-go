package cachestore

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// KeyPrefixLabel is the label for cache metrics, representing the key prefix.
	KeyPrefixLabel = "key_prefix"
)

// Metrics contains the Prometheus collectors for cache-related metrics.
type Metrics struct {
	Hits   *prometheus.CounterVec
	Misses *prometheus.CounterVec
	Size   *prometheus.GaugeVec
}

// NewMetrics creates and registers the cache metrics collectors.
// It returns an error if any of the collectors fail to register.
func NewMetrics() Metrics {
	m := Metrics{
		Hits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_hit_count",
			Help: "The number of cache hits",
		}, []string{KeyPrefixLabel}),
		Misses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_miss_count",
			Help: "The number of cache misses",
		}, []string{KeyPrefixLabel}),
		Size: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "The size of a set within the cache, identified by its key",
		}, []string{KeyPrefixLabel}),
	}
	prometheus.MustRegister(
		m.Hits,
		m.Misses,
		m.Size,
	)
	return m
}
