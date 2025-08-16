package main

import (
	"context"
	_ "embed"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hypedn/mflag"
	"github.com/ndajr/urlshortener-go/internal/cachestore"
	"github.com/ndajr/urlshortener-go/internal/config"
	"github.com/ndajr/urlshortener-go/internal/datastore"
	"github.com/ndajr/urlshortener-go/internal/httpserver"
	"github.com/ndajr/urlshortener-go/internal/rpcserver"
)

var (
	version   = "dev"
	gitCommit = "none"
)

//go:embed apidocs.swagger.json
var swaggerJSON []byte

func main() {
	config.SetDefaults()
	if err := mflag.Init("configmap.yaml"); err != nil {
		log.Fatal(err)
	}
	mflag.Parse()

	ctx, shutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer shutdown()

	appCfg, redisCfg, rlCfg := config.GetSettings()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("starting urlshortener service", "version", version, "commit", gitCommit)

	db, err := datastore.NewStore(ctx, logger, appCfg)
	if err != nil {
		logger.Error("failed to connect to datastore", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	cache, err := cachestore.NewCache(ctx, logger, redisCfg)
	if err != nil {
		logger.Error("failed to connect to cache", "error", err)
		os.Exit(1)
	}
	defer cache.Close()

	var wg sync.WaitGroup

	grpcSrv := rpcserver.NewServer(logger, db, cache, &rlCfg)
	if runErr := grpcSrv.Run(ctx, appCfg.GrpcEndpoint, &wg); runErr != nil {
		logger.Error("failed to run gRPC server", "error", runErr)
		os.Exit(1)
	}

	gwmux := grpcSrv.NewGatewayMux()
	httpSrv := httpserver.NewServer(grpcSrv, gwmux, logger, swaggerJSON)
	if runErr := httpSrv.Run(ctx, appCfg.HttpEndpoint, &wg); runErr != nil {
		logger.Error("failed to run HTTP server", "error", runErr)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("powering down urlshortener service")
	wg.Wait()
}
