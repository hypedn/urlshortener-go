package rpcserver

import (
	"context"
	"errors"
	"log/slog"
	"net"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ndajr/urlshortener-go/datastore"
	proto "github.com/ndajr/urlshortener-go/rpcserver/proto/urlshortener/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server struct {
	logger     *slog.Logger
	grpcServer *grpc.Server
	endpoint   string

	URLShorteningService URLShortenerService
}

func NewServer(db datastore.Store, logger *slog.Logger, grpcAddress string) Server {
	grpcServer := grpc.NewServer()
	grpc_prometheus.Register(grpcServer)

	srv := Server{
		logger:               logger,
		grpcServer:           grpcServer,
		endpoint:             grpcAddress,
		URLShorteningService: NewURLShortenerService(db, logger),
	}

	srv.registerServices(grpcServer)
	return srv
}

func (s *Server) registerServices(srv *grpc.Server) {
	proto.RegisterURLShortenerServiceServer(srv, s.URLShorteningService)
}

func (s *Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.endpoint)
	if err != nil {
		return err
	}

	go func() {
		s.logger.Info("starting urlshortener gRPC service", "addr", s.endpoint)
		if err := s.grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			s.logger.Error("gRPC server failed to serve", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		s.logger.Info("gRPC server shutting down")
		s.grpcServer.GracefulStop()
	}()

	return nil
}

func (s *Server) NewGatewayMux(ctx context.Context) (*runtime.ServeMux, error) {
	gwmux := runtime.NewServeMux(runtime.WithErrorHandler(CustomHTTPErrorHandler))
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err := proto.RegisterURLShortenerServiceHandlerFromEndpoint(ctx, gwmux, s.endpoint, opts)
	if err != nil {
		return nil, err
	}
	return gwmux, nil
}
