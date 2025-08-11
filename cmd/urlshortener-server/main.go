package main

import (
	"context"
	_ "embed"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ndajr/urlshortener-go/internal/cachestore"
	"github.com/ndajr/urlshortener-go/internal/datastore"
	"github.com/ndajr/urlshortener-go/internal/httpserver"
	"github.com/ndajr/urlshortener-go/internal/rpcserver"
)

var (
	httpServerEndpoint = flag.String("http-server-endpoint", "localhost:8080", "http server endpoint")
	grpcServerEndpoint = flag.String("grpc-server-endpoint", "localhost:8081", "gRPC server endpoint")
	dbAddr             = flag.String("db-addr", "postgres://ndev:@localhost:5432/urlshortener?sslmode=disable", "database DSN")
	redisAddr          = flag.String("redis-addr", "localhost:6379", "redis host")
)

//go:embed apidocs.swagger.json
var swaggerJSON []byte

func main() {
	flag.Parse()

	ctx, shutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer shutdown()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := datastore.NewStore(ctx, logger, *dbAddr)
	if err != nil {
		logger.Error("failed to connect to datastore", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	cache, err := cachestore.NewCache(ctx, *redisAddr, logger)
	if err != nil {
		logger.Error("failed to connect to cache", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	var wg sync.WaitGroup

	grpcSrv := rpcserver.NewServer(logger, db, cache)
	if runErr := grpcSrv.Run(ctx, *grpcServerEndpoint, &wg); runErr != nil {
		logger.Error("failed to run gRPC server", "error", runErr)
		os.Exit(1)
	}

	gwmux := grpcSrv.NewGatewayMux()
	httpSrv := httpserver.NewServer(grpcSrv, gwmux, logger, swaggerJSON)
	if runErr := httpSrv.Run(ctx, *httpServerEndpoint, &wg); runErr != nil {
		logger.Error("failed to run HTTP server", "error", runErr)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("powering down urlshortener service")
	wg.Wait()
}
