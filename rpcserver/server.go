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
	gwmux      *runtime.ServeMux

	URLShorteningService URLShortenerService
}

func NewServer(db datastore.Store, logger *slog.Logger) Server {
	grpcServer := grpc.NewServer()
	grpc_prometheus.Register(grpcServer)

	srv := Server{
		logger:               logger,
		grpcServer:           grpcServer,
		URLShorteningService: NewURLShortenerService(db, logger),
	}

	srv.registerServices(grpcServer)
	return srv
}

func (s *Server) registerServices(srv *grpc.Server) {
	proto.RegisterURLShortenerServiceServer(srv, s.URLShorteningService)
}

func (s *Server) Run(ctx context.Context, address string) error {
	conn, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	go func() {
		s.logger.Info("starting urlshortener gRPC service", "addr", address)
		err = s.grpcServer.Serve(conn)
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			s.logger.Error("gRPC server failed to serve", "error", err)
		}
	}()

	gwmux := runtime.NewServeMux(runtime.WithErrorHandler(CustomHTTPErrorHandler))
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err = proto.RegisterURLShortenerServiceHandlerFromEndpoint(ctx, gwmux, address, opts)
	if err != nil {
		return err
	}
	s.gwmux = gwmux

	go func() {
		<-ctx.Done()
		s.logger.Info("gRPC server shutting down")
		s.grpcServer.GracefulStop()
		if err := conn.Close(); err != nil {
			s.logger.Error("gRPC server graceful shutdown failed", "error", err)
		}
	}()

	return nil
}

func (s *Server) NewGatewayMux() *runtime.ServeMux {
	return s.gwmux
}
