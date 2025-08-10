package rpcserver

import (
	"context"
	"fmt"

	"github.com/ndajr/urlshortener-go/internal/cachestore"
	"github.com/ndajr/urlshortener-go/internal/datastore"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var _ healthpb.HealthServer = (*HealthService)(nil)

type HealthService struct {
	healthpb.UnimplementedHealthServer
	db    datastore.Store
	cache *cachestore.Cache
}

func NewHealthService(db datastore.Store, cache *cachestore.Cache) HealthService {
	return HealthService{
		db:    db,
		cache: cache,
	}
}

func (h HealthService) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if err := h.up(ctx); err != nil {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, err
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (h HealthService) up(ctx context.Context) error {
	if err := h.db.Ping(ctx); err != nil {
		return fmt.Errorf("health: db not ok")
	}
	if h.cache != nil {
		if err := h.cache.Ping(ctx); err != nil {
			return fmt.Errorf("health: redis not ok")
		}
	}
	return nil
}
