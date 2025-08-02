package systemtest

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ndajr/urlshortener-go/datastore"
	"github.com/ndajr/urlshortener-go/rpc"
	proto "github.com/ndajr/urlshortener-go/rpc/proto/urlshortener/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	client proto.URLShortenerServiceClient
)

const (
	dbAddr       = "postgres://ndev@localhost:5432/urlshortener?sslmode=disable"
	grpcTestAddr = "localhost:50051"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := datastore.NewStore(ctx, logger, dbAddr, "")
	if err != nil {
		logger.Error("datastore was unable to start", "error", err)
		os.Exit(1)
	}

	grpcServer := rpc.NewServer(db, logger)

	go func() {
		_, err := grpcServer.Run(ctx, grpcTestAddr)
		if err != nil {
			logger.Error("gRPC server failed during test", "error", err)
			os.Exit(1)
		}
	}()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient(grpcTestAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("failed to connect to gRPC server", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	client = proto.NewURLShortenerServiceClient(conn)

	code := m.Run()
	os.Exit(code)
}
