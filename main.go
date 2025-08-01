package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ndajr/urlshortener-go/datastore"
	"github.com/ndajr/urlshortener-go/rpc"
	swgui "github.com/swaggest/swgui/v5"
)

const docsURL = "/docs"

var (
	httpServerEndpoint = flag.String("http-server-endpoint", "localhost:8080", "http server endpoint")
	grpcServerEndpoint = flag.String("grpc-server-endpoint", "localhost:8081", "gRPC server endpoint")
	dbAddr             = flag.String("db-addr", "postgres://ndev:@localhost:5432/urlshortener?sslmode=disable", "database DSN")
)

//go:embed apidocs.swagger.json
var swaggerJSON []byte

func main() {
	flag.Parse()

	ctx, shutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer shutdown()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := datastore.NewStore(ctx, *dbAddr, logger)
	if err != nil {
		logger.Error("failed to connect to datastore", "error", err)
		shutdown()
	}
	defer db.Close()

	logger.Info("starting urlshortener grpc service", "port", *grpcServerEndpoint)
	srv := rpc.NewServer(db, logger)
	gwmux, err := srv.Run(ctx, *grpcServerEndpoint)
	if err != nil {
		logger.Error("failed to run gRPC server", "error", err)
		shutdown()
	}
	defer srv.Stop()

	mux := http.NewServeMux()
	mux.Handle("/", gwmux)

	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(swaggerJSON)
		if err != nil {
			logger.Error("failed to respond with swagger.json content", "error", err)
		}
	})
	swaggerHandler := swgui.NewHandler("URL Shortener API", "/swagger.json", docsURL)
	mux.Handle(docsURL, swaggerHandler)

	httpSrv := &http.Server{
		Addr:    *httpServerEndpoint,
		Handler: mux,
	}

	startHTTPServer(httpSrv, logger, shutdown)
	defer shutdownHTTPServer(httpSrv, logger)

	<-ctx.Done()
	logger.Info("powering down urlshortener service")
}

func startHTTPServer(srv *http.Server, logger *slog.Logger, shutdown context.CancelFunc) {
	logger.Info("starting urlshortener http service", "port", srv.Addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			shutdown()
		}
	}()
}

func shutdownHTTPServer(srv *http.Server, logger *slog.Logger) {
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to gracefully shutdown HTTP server", "error", err)
		os.Exit(1)
	}
}
