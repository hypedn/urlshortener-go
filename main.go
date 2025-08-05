package main

import (
	"context"
	_ "embed"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ndajr/urlshortener-go/datastore"
	"github.com/ndajr/urlshortener-go/httpserver"
	"github.com/ndajr/urlshortener-go/rpcserver"
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

	db, err := datastore.NewStore(ctx, logger, *dbAddr, *redisAddr)
	if err != nil {
		logger.Error("failed to connect to datastore", "error", err)
		return
	}
	defer db.Close()

	grpcSrv := rpcserver.NewServer(db, logger)
	err = grpcSrv.Run(ctx, *grpcServerEndpoint)
	if err != nil {
		logger.Error("failed to run gRPC server", "error", err)
		return
	}

	gwmux := grpcSrv.NewGatewayMux()
	httpSrv := httpserver.NewServer(grpcSrv, gwmux, logger, swaggerJSON)
	if err := httpSrv.Run(ctx, *httpServerEndpoint); err != nil {
		logger.Error("failed to run HTTP server", "error", err)
		return
	}

	<-ctx.Done()
	logger.Info("powering down urlshortener service")
}
