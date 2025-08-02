package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxv5 "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ndajr/urlshortener-go/core"
	"github.com/redis/go-redis/v9"
)

var (
	ErrFailedToAddURL = errors.New("failed to add url")
	ErrURLNotFound    = errors.New("url not found")
)

const (
	// maxRetries is the number of times to retry generating a unique short code.
	maxRetries = 5
	// dbConnectTimeout is the timeout for establishing a database connection.
	dbConnectTimeout = 15 * time.Second
	// cacheTTL is the time-to-live for cached URL entries.
	cacheTTL = 1 * time.Hour
)

type Store struct {
	db        *pgxpool.Pool
	logger    *slog.Logger
	cache     Cache
	dbMetrics *DBMetrics
}

// NewStore establishes a database connection and returns a new Store.
func NewStore(ctx context.Context, logger *slog.Logger, dbConnStr, redisConnStr string) (Store, error) {
	ctx, cancel := context.WithTimeout(ctx, dbConnectTimeout)
	defer cancel()

	db, err := pgxpool.New(ctx, dbConnStr)
	if err != nil {
		return Store{}, fmt.Errorf("store: failed to create connection pool: %w", err)
	}

	err = Ping(ctx, db, logger)
	if err != nil {
		return Store{}, err
	}

	err = runMigrations(dbConnStr)
	if err != nil {
		return Store{}, fmt.Errorf("store: failed to run migrations: %w", err)
	}
	logger.Info("successfully connected to db", "addr", dbConnStr)

	// Parse the DSN to get the database name for use as a Prometheus label.
	config, err := pgxpool.ParseConfig(dbConnStr)
	if err != nil {
		db.Close()
		return Store{}, fmt.Errorf("store: failed to parse db config for metrics: %w", err)
	}
	dbName := config.ConnConfig.Database

	dbMetrics, err := NewDBMetrics(db, dbName)
	if err != nil {
		db.Close()
		return Store{}, fmt.Errorf("store: failed to create db metrics: %w", err)
	}

	store := Store{
		db:        db,
		logger:    logger,
		dbMetrics: dbMetrics,
	}

	if redisConnStr != "" {
		cache, err := NewCache(ctx, redisConnStr, logger)
		if err != nil {
			db.Close() // clean up db connection on failure
			return Store{}, fmt.Errorf("store: failed to connect to cache: %w", err)
		}
		store.cache = cache
	}

	return store, nil
}

func runMigrations(connStr string) (err error) {
	migrationDB, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("store: failed to open migration db: %w", err)
	}
	defer func() {
		err = migrationDB.Close()
	}()

	driver, err := pgxv5.WithInstance(migrationDB, &pgxv5.Config{})
	if err != nil {
		return fmt.Errorf("store: failed to create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://.migrations",
		"pgx",
		driver,
	)
	if err != nil {
		return fmt.Errorf("store: failed to create migrate instance: %w", err)
	}
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("store: failed to run migrations: %w", err)
	}
	return nil
}

// AddURL generates a short code for a URL and stores it in the database.
// It retries on collision.
func (s Store) AddURL(ctx context.Context, longURL string) (core.URL, error) {
	const queryName = "AddURL"

	for i := 0; i < maxRetries; i++ {
		shortCode, err := core.GenerateShortCode()
		if err != nil {
			return core.URL{}, fmt.Errorf("store: %w", err)
		}

		start := time.Now()
		rows, err := s.db.Query(ctx, insertURL, pgx.NamedArgs{
			"short_code": shortCode,
			"long_url":   longURL,
		})
		if err != nil {
			s.dbMetrics.QueryDuration.WithLabelValues(queryName).Observe(time.Since(start).Seconds())
			s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusError).Inc()
			return core.URL{}, fmt.Errorf("store: insertURL: %w", err)
		}

		out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[core.URL])
		s.dbMetrics.QueryDuration.WithLabelValues(queryName).Observe(time.Since(start).Seconds())

		if err == nil {
			s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusSuccess).Inc()
			return out, nil
		}

		if errors.Is(err, pgx.ErrNoRows) {
			// pgx.ErrNoRows is expected on a key collision, so we log and retry.
			s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusCollision).Inc()
			s.logger.Info("collision detected, generating a new short code", "short_code", shortCode)
		} else {
			s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusError).Inc()
			return core.URL{}, fmt.Errorf("store: failed to collect inserted row: %w", err)
		}
	}

	return core.URL{}, fmt.Errorf("store: %w", ErrFailedToAddURL)
}

// GetURL retrieves the original long URL for a given short code.
func (s Store) GetURL(ctx context.Context, shortCode string) (string, error) {
	// Check cache if the redis client is initialized.
	if s.cache.rdb != nil {
		longURL, err := s.cache.Get(ctx, shortCode)
		if err == nil {
			return longURL, nil // Cache hit
		}
		// If it's any error other than "not found", log it but proceed to DB.
		if !errors.Is(err, redis.Nil) {
			s.logger.Error("redis cache Get failed", "error", err)
		}
	}

	const queryName = "GetURL"
	start := time.Now()
	defer func() {
		s.dbMetrics.QueryDuration.WithLabelValues(queryName).Observe(time.Since(start).Seconds())
	}()

	rows, err := s.db.Query(ctx, getURL, shortCode)
	if err != nil {
		s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusError).Inc()
		return "", fmt.Errorf("store: GetURL: %w", err)
	}

	longURL, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[string])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The query was successful but found no rows. This is not a DB error.
			s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusSuccess).Inc()
			return "", ErrURLNotFound
		}
		// Any other error from CollectExactlyOneRow is a DB error.
		s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusError).Inc()
		return "", fmt.Errorf("store: GetURL: %w", err)
	}

	// Success
	s.dbMetrics.QueryTotal.WithLabelValues(queryName, StatusSuccess).Inc()

	// After a successful DB lookup, store the result in the cache for future requests.
	if s.cache.rdb != nil {
		err := s.cache.Set(ctx, shortCode, longURL, cacheTTL)
		if err != nil {
			// Log the error but don't fail the whole operation, as the primary goal was met.
			s.logger.Error("redis cache Set failed", "error", err)
		}
	}

	return longURL, nil
}

func (s Store) Close() {
	if s.cache.rdb != nil {
		s.cache.Close()
	}
	s.db.Close()
}
