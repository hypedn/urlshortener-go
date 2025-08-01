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
	db *pgxpool.Pool
}

// NewStore establishes a database connection and returns a new Store.
func NewStore(parentCtx context.Context, connStr string, logger *slog.Logger) (Store, error) {
	ctx, cancel := context.WithTimeout(parentCtx, dbConnectTimeout)
	defer cancel()

	db, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return Store{}, fmt.Errorf("store: failed to create connection pool: %w", err)
	}

	if err := checkDBConnection(ctx, db, logger); err != nil {
		return Store{}, err
	}

	err = runMigrations(connStr)
	if err != nil {
		return Store{}, fmt.Errorf("store: failed to run migrations: %w", err)
	}
	return Store{db: db}, nil
}

func checkDBConnection(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) (err error) {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Loop until the context is cancelled or the ping is successful.
	for {
		err = db.Ping(ctx)
		if err == nil {
			break // Ping successful.
		}

		logger.Warn("unable to establish postgres db connection, retrying...", "error", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("db connection timed out or was cancelled: %w (last error: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
	}
	return nil
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
	for i := 0; i < maxRetries; i++ {
		shortCode, err := core.GenerateShortCode()
		if err != nil {
			return core.URL{}, fmt.Errorf("store: %w", err)
		}

		rows, err := s.db.Query(ctx, insertURL, pgx.NamedArgs{
			"short_code": shortCode,
			"long_url":   longURL,
		})
		if err != nil {
			return core.URL{}, fmt.Errorf("store: insertURL: %w", err)
		}

		out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[core.URL])
		if err == nil {
			return out, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return core.URL{}, fmt.Errorf("store: failed to collect inserted row: %w", err)
		}

		// pgx.ErrNoRows is expected on a key collision, so we log and retry.
		slog.Info("collision detected, generating a new short code", "short_code", shortCode)
	}

	return core.URL{}, fmt.Errorf("store: %w", ErrFailedToAddURL)
}

// GetURL retrieves the original long URL for a given short code.
func (s Store) GetURL(ctx context.Context, shortCode string) (string, error) {
	rows, err := s.db.Query(ctx, getURL, shortCode)
	if err != nil {
		return "", fmt.Errorf("store: GetURL: %w", err)
	}

	longURL, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[string])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrURLNotFound
		}
		return "", fmt.Errorf("store: GetURL: %w", err)
	}

	return longURL, nil
}

func (s Store) Close() {
	s.db.Close()
}
