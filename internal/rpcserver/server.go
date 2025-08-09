package rpcserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ndajr/urlshortener-go/internal/cachestore"
	"github.com/ndajr/urlshortener-go/internal/datastore"
	proto "github.com/ndajr/urlshortener-go/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server struct {
	logger     *slog.Logger
	grpcServer *grpc.Server
	gwmux      *runtime.ServeMux

	URLShorteningService URLShortenerService
}

func NewServer(logger *slog.Logger, db datastore.Store, cache *cachestore.Cache) Server {
	config := cachestore.RateLimiterConfig{
		Capacity:     10,          // 10 token burst
		RefillRate:   40,          // 40 tokens per period
		RefillPeriod: time.Second, // Every second
	}

	opts := []grpc.ServerOption{}
	if cache != nil {
		limiter := cachestore.NewRateLimiter(logger, cache, config)
		opts = append(opts, grpc.UnaryInterceptor(limiter.UnaryServerInterceptor()))
	}
	grpcServer := grpc.NewServer(opts...)
	grpc_prometheus.Register(grpcServer)

	srv := Server{
		logger:               logger,
		grpcServer:           grpcServer,
		URLShorteningService: NewURLShortenerService(logger, db, cache),
	}

	srv.registerServices(grpcServer)
	return srv
}

func (s *Server) registerServices(srv *grpc.Server) {
	proto.RegisterURLShortenerServiceServer(srv, s.URLShorteningService)
}

func (s *Server) Run(ctx context.Context, address string, wg *sync.WaitGroup) error {
	conn, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	go func() {
		s.logger.Info("starting urlshortener gRPC service", "addr", address)
		if serveErr := s.grpcServer.Serve(conn); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
			s.logger.Error("gRPC server failed to serve", "error", serveErr)
		}
	}()

	gwmux := runtime.NewServeMux(runtime.WithErrorHandler(NewCustomHTTPErrorHandler(s.logger)))
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	gwConn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return err
	}

	err = proto.RegisterURLShortenerServiceHandler(ctx, gwmux, gwConn)
	if err != nil {
		if closeErr := gwConn.Close(); closeErr != nil {
			s.logger.Error("failed to close gateway client connection after registration error", "error", closeErr)
		}
		return err
	}
	s.gwmux = gwmux

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		s.logger.Info("gRPC server shutting down")
		s.grpcServer.GracefulStop()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		s.logger.Info("gRPC gateway client shutting down")
		if closeErr := gwConn.Close(); closeErr != nil {
			s.logger.Error("gRPC gateway client shutdown failed", "error", closeErr)
		}
	}()

	return nil
}

func (s *Server) NewGatewayMux() *runtime.ServeMux {
	return s.gwmux
}
