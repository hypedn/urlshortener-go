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
	"github.com/ndajr/urlshortener-go/rpcserver"
	swaggerui "github.com/swaggest/swgui/v5emb"
)

const docsURL = "/docs/"

type Server struct {
	server     rpcserver.Server
	httpServer *http.Server
	logger     *slog.Logger
}

func NewServer(server rpcserver.Server, gwmux *runtime.ServeMux, logger *slog.Logger, httpAddress string, swaggerJSON []byte) *Server {
	s := &Server{
		server: server,
		logger: logger,
	}
	httpSrv := &http.Server{
		Addr:    httpAddress,
		Handler: s.registerEndpoints(gwmux, swaggerJSON),
	}
	s.httpServer = httpSrv
	return s
}

func (s *Server) registerEndpoints(gwmux *runtime.ServeMux, swaggerJSON []byte) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/api/", gwmux)
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(swaggerJSON)
		if err != nil {
			s.logger.Error("failed to respond with swagger.json content", "error", err)
			return
		}
	})
	mux.Handle(docsURL, swaggerui.New("URL Shortener API", "/swagger.json", docsURL))
	mux.HandleFunc("/", s.redirectHandler())

	return mux
}

func (s Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.httpServer.Addr, err)
	}

	go func() {
		s.logger.Info("starting urlshortener http service", "addr", s.httpServer.Addr)
		if err := s.httpServer.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http server failed to serve", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		s.logger.Info("http server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("http server graceful shutdown failed", "error", err)
		}
	}()

	return nil
}
