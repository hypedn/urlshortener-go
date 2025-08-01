package rpc

import (
	"context"
	"log/slog"
	"net"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ndajr/urlshortener-go/datastore"
	proto "github.com/ndajr/urlshortener-go/rpc/proto/urlshortener/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server struct {
	logger     *slog.Logger
	grpcServer *grpc.Server

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

func (s Server) registerServices(srv *grpc.Server) {
	proto.RegisterURLShortenerServiceServer(srv, s.URLShorteningService)
}

func (s Server) Run(ctx context.Context, grpcAddress string) (gwmux *runtime.ServeMux, err error) {
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		return nil, err
	}

	go func() {
		if serveErr := s.grpcServer.Serve(lis); serveErr != nil {
			s.logger.Error("gRPC server failed", "error", serveErr)
			s.Stop()
		}
	}()

	gwmux = runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err = proto.RegisterURLShortenerServiceHandlerFromEndpoint(ctx, gwmux, grpcAddress, opts)
	if err != nil {
		return nil, err
	}
	return gwmux, nil
}

func (s Server) Stop() {
	s.grpcServer.GracefulStop()
}
