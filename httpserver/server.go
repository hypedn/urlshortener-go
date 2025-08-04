package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	swaggerui "github.com/swaggest/swgui/v5emb"
)

const docsURL = "/docs/"

type Server struct {
	server *http.Server
	logger *slog.Logger
}

func NewServer(gwmux *runtime.ServeMux, logger *slog.Logger, httpAddress string, swaggerJSON []byte) *Server {
	server := &http.Server{
		Addr:    httpAddress,
		Handler: registerEndpoints(gwmux, logger, swaggerJSON),
	}
	return &Server{server: server, logger: logger}
}

func registerEndpoints(gwmux *runtime.ServeMux, logger *slog.Logger, swaggerJSON []byte) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/api/", gwmux)
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(swaggerJSON)
		if err != nil {
			logger.Error("failed to respond with swagger.json content", "error", err)
			return
		}
	})
	mux.Handle(docsURL, swaggerui.New("URL Shortener API", "/swagger.json", docsURL))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, docsURL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	return mux
}

func (s Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.server.Addr, err)
	}

	go func() {
		s.logger.Info("starting urlshortener http service", "addr", s.server.Addr)
		if err := s.server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http server failed to serve", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		s.logger.Info("http server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("http server graceful shutdown failed", "error", err)
		}
	}()

	return nil
}
