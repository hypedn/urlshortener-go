package datastore

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	DBNameLabel = "db_name"
	// QueryNameLabel is the label for DB metrics, representing the query name (e.g., "AddURL", "GetURL").
	QueryNameLabel = "query_name"
	// StatusLabel is the label for DB metrics, representing the outcome (e.g., "success", "error").
	StatusLabel = "status"

	// StatusSuccess is the label for a successful operation.
	StatusSuccess = "success"
	// StatusError is the label for a failed operation.
	StatusError = "error"
	// StatusCollision is the label for a key collision during an insert.
	StatusCollision = "collision"
)

// DBMetrics contains the Prometheus collectors for application-specific database metrics.
// Pool-level stats are handled by the separate PoolStatsCollector.
type DBMetrics struct {
	QueryDuration *prometheus.HistogramVec
	QueryTotal    *prometheus.CounterVec
}

type StatsCollector interface {
	Stat() *pgxpool.Stat
}

// PoolStatsCollector collects pgxpool.Stat metrics for Prometheus.
// It implements the prometheus.Collector interface.
type PoolStatsCollector struct {
	db StatsCollector

	// Descriptions of the metrics, which are initialized once and reused.
	MaxConns           *prometheus.Desc
	TotalConns         *prometheus.Desc
	AcquiredConns      *prometheus.Desc
	IdleConns          *prometheus.Desc
	AcquireCount       *prometheus.Desc
	AcquireDuration    *prometheus.Desc
	MaxIdleDestroy     *prometheus.Desc
	MaxLifetimeDestroy *prometheus.Desc
}

// NewPoolStatsCollector creates a new PoolStatsCollector.
func NewPoolStatsCollector(db StatsCollector, dbName string) *PoolStatsCollector {
	return &PoolStatsCollector{
		db: db,
		MaxConns: prometheus.NewDesc(
			"db_pool_max_conns",
			"Maximum number of connections in the pool.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		TotalConns: prometheus.NewDesc(
			"db_pool_total_conns",
			"Total number of connections in the pool.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		AcquiredConns: prometheus.NewDesc(
			"db_pool_acquired_conns",
			"Number of currently acquired connections in the pool.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		IdleConns: prometheus.NewDesc(
			"db_pool_idle_conns",
			"Number of currently idle connections in the pool.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		AcquireCount: prometheus.NewDesc(
			"db_pool_acquire_count_total",
			"Cumulative count of successful connection acquisitions.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		AcquireDuration: prometheus.NewDesc(
			"db_pool_acquire_duration_seconds_total",
			"Total time blocked waiting for a new connection, in seconds.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		MaxIdleDestroy: prometheus.NewDesc(
			"db_pool_max_idle_closed_total",
			"Cumulative count of connections closed due to MaxIdleConns.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
		MaxLifetimeDestroy: prometheus.NewDesc(
			"db_pool_max_lifetime_closed_total",
			"Cumulative count of connections closed due to MaxConnLifetime.",
			nil, prometheus.Labels{DBNameLabel: dbName},
		),
	}
}

// Describe implements the prometheus.Collector interface. It sends the descriptions
// of all metrics collected by this collector to the provided channel.
func (c *PoolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.MaxConns
	ch <- c.TotalConns
	ch <- c.AcquiredConns
	ch <- c.IdleConns
	ch <- c.AcquireCount
	ch <- c.AcquireDuration
	ch <- c.MaxIdleDestroy
	ch <- c.MaxLifetimeDestroy
}

// Collect implements the prometheus.Collector interface. It is called by Prometheus
// to gather metrics.
func (c *PoolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.db.Stat()
	ch <- prometheus.MustNewConstMetric(c.MaxConns, prometheus.GaugeValue, float64(stats.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.TotalConns, prometheus.GaugeValue, float64(stats.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.AcquiredConns, prometheus.GaugeValue, float64(stats.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.IdleConns, prometheus.GaugeValue, float64(stats.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.AcquireCount, prometheus.CounterValue, float64(stats.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.AcquireDuration, prometheus.CounterValue, stats.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.MaxIdleDestroy, prometheus.CounterValue, float64(stats.MaxIdleDestroyCount()))
	ch <- prometheus.MustNewConstMetric(c.MaxLifetimeDestroy, prometheus.CounterValue, float64(stats.MaxLifetimeDestroyCount()))
}

// NewDBMetrics creates and registers the database metrics collectors.
// It returns an error if any of the collectors fail to register.
func NewDBMetrics(db StatsCollector, dbName string) (*DBMetrics, error) {
	m := &DBMetrics{
		QueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "The latency of database queries in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		}, []string{QueryNameLabel}),

		QueryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "db_query_total",
			Help: "The total number of database queries.",
		}, []string{QueryNameLabel, StatusLabel}),
	}

	// Register the application-specific metrics that are manually updated.
	collectors := []prometheus.Collector{
		m.QueryDuration,
		m.QueryTotal,
	}
	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			return nil, err
		}
	}

	// Register the pool stats collector, which will be scraped on-demand.
	poolCollector := NewPoolStatsCollector(db, dbName)
	if err := prometheus.Register(poolCollector); err != nil {
		return nil, err
	}

	return m, nil
}
