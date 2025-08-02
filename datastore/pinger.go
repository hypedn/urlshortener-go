package datastore

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func Ping(ctx context.Context, pinger Pinger, logger *slog.Logger) (err error) {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	// Loop until the context is cancelled or the ping is successful.
	for {
		err = pinger.Ping(ctx)
		if err == nil {
			break // Ping successful.
		}

		logger.Warn("unable to establish connection, retrying...", "error", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("db connection timed out or was cancelled: %w (last error: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
	}
	return nil
}
