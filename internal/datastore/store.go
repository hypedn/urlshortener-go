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
	"github.com/ndajr/urlshortener-go/internal/core"
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
)

type Store struct {
	db        *pgxpool.Pool
	logger    *slog.Logger
	dbMetrics Metrics
}

// NewStore establishes a database connection and returns a new Store.
func NewStore(ctx context.Context, logger *slog.Logger, dbConnStr string) (Store, error) {
	ctx, cancel := context.WithTimeout(ctx, dbConnectTimeout)
	defer cancel()

	db, err := pgxpool.New(ctx, dbConnStr)
	if err != nil {
		return Store{}, fmt.Errorf("store: failed to create connection pool: %w", err)
	}

	config, err := pgxpool.ParseConfig(dbConnStr)
	if err != nil {
		db.Close()
		return Store{}, fmt.Errorf("store: failed to parse db config for metrics: %w", err)
	}

	store := Store{
		db:        db,
		logger:    logger,
		dbMetrics: NewMetrics(db, config.ConnConfig.Database),
	}

	if pingErr := store.Ping(ctx); pingErr != nil {
		return Store{}, pingErr
	}

	if migrErr := runMigrations(dbConnStr); migrErr != nil {
		db.Close()
		return Store{}, fmt.Errorf("store: failed to run migrations: %w", migrErr)
	}
	logger.Info("successfully connected to db", "addr", dbConnStr)

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
	if runErr := m.Up(); runErr != nil && !errors.Is(runErr, migrate.ErrNoChange) {
		return fmt.Errorf("store: failed to run migrations: %w", runErr)
	}
	return nil
}

func (s Store) Ping(ctx context.Context) error {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Loop until the context is cancelled or the ping is successful.
	for {
		err := s.db.Ping(ctx)
		if err == nil {
			break // Ping successful.
		}

		s.logger.Warn("unable to establish connection, retrying...", "error", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("db connection timed out or was cancelled: %w (last error: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
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

	return longURL, nil
}

func (s Store) Close() {
	s.db.Close()
}
