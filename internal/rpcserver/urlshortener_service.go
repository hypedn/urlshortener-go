package rpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/ndajr/urlshortener-go/internal/cachestore"
	"github.com/ndajr/urlshortener-go/internal/core"
	"github.com/ndajr/urlshortener-go/internal/datastore"
	proto "github.com/ndajr/urlshortener-go/proto/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrStoreInternal         = errors.New("internal error")
	ErrStoreDeadlineExceeded = errors.New("the request has timed out, please try again")
	ErrStoreInvalidRequest   = errors.New("invalid request or missing data")
	ErrStoreURLNotFound      = errors.New("url not found")
)

type URLShortenerService struct {
	proto.UnimplementedURLShortenerServiceServer
	db     datastore.Store
	cache  *cachestore.Cache
	logger *slog.Logger
}

var _ proto.URLShortenerServiceServer = (*URLShortenerService)(nil)

func NewURLShortenerService(logger *slog.Logger, db datastore.Store, cache *cachestore.Cache) URLShortenerService {
	return URLShortenerService{
		logger: logger,
		db:     db,
		cache:  cache,
	}
}

func (s URLShortenerService) GetOriginalURL(ctx context.Context, req *proto.GetOriginalURLRequest) (*proto.GetOriginalURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "missing short code")
	}
	url, err := s.getCached(ctx, req.ShortCode)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return s.loadCache(ctx, req.ShortCode)
		}
		s.logger.Warn("cache lookup failed, falling back to database", "shortCode", req.ShortCode, "error", err)
		return s.loadCache(ctx, req.ShortCode)
	}
	return url, nil
}

func (s URLShortenerService) getCached(ctx context.Context, shortCode string) (*proto.GetOriginalURLResponse, error) {
	if s.cache == nil {
		return nil, redis.Nil
	}

	url, err := s.cache.GetURL(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	return &proto.GetOriginalURLResponse{OriginalUrl: url}, nil
}

func (s URLShortenerService) loadCache(ctx context.Context, shortCode string) (*proto.GetOriginalURLResponse, error) {
	url, err := s.db.GetURL(ctx, shortCode)
	if err != nil {
		if errors.Is(err, datastore.ErrURLNotFound) {
			return nil, status.Error(codes.NotFound, ErrStoreURLNotFound.Error())
		}
		s.logger.Error("failed to read url from db", "shortCode", shortCode, "error", err)
		return nil, status.Error(codes.Internal, ErrStoreInternal.Error())
	}

	if s.cache == nil {
		return &proto.GetOriginalURLResponse{OriginalUrl: url}, nil
	}

	go func() {
		bgCtx := context.WithoutCancel(ctx)
		bgCtx, cancel := context.WithTimeout(bgCtx, 2*time.Second)
		defer cancel()
		if err := s.cache.SetURL(bgCtx, shortCode, url); err != nil {
			s.logger.Error("Failed to update cache in background", "key", shortCode, "error", err)
		}
	}()

	return &proto.GetOriginalURLResponse{OriginalUrl: url}, nil
}

func (s URLShortenerService) ShortenURL(ctx context.Context, req *proto.ShortenURLRequest) (*proto.ShortenURLResponse, error) {
	parsedURL, err := parseURL(req.OriginalUrl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	url, err := s.db.AddURL(ctx, parsedURL)
	if err != nil {
		if errors.Is(err, datastore.ErrFailedToAddURL) {
			return nil, status.Error(codes.DeadlineExceeded, ErrStoreDeadlineExceeded.Error())
		}
		s.logger.Error("ShortenURL internal error", "error", err)
		return nil, status.Error(codes.Internal, ErrStoreInternal.Error())
	}
	return &proto.ShortenURLResponse{ShortCode: url.ShortCode}, nil
}

func parseURL(originalURL string) (string, error) {
	originalURL = strings.TrimSpace(originalURL)
	if originalURL == "" {
		return "", fmt.Errorf("missing original url")
	}

	if len(originalURL) > core.MaxURLLength {
		return "", fmt.Errorf("url exceeds maximum length of %d characters", core.MaxURLLength)
	}

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return "", fmt.Errorf("invalid url format: %w", err)
	}

	// We only accept absolute URLs with http or https schemes.
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("only http and https schemes are accepted")
	}

	// The `//` check is to prevent open redirects like `//example.com`.
	// The `..` check is to prevent path traversal attacks.
	if strings.Contains(parsedURL.Path, "..") || strings.Contains(parsedURL.Path, "//") {
		return "", fmt.Errorf("potentially unsafe url path")
	}

	if isLocalhost(parsedURL.Host) {
		return "", fmt.Errorf("localhost and internal addresses not allowed")
	}

	return parsedURL.String(), nil
}

// isLocalhost checks if a given host string represents a local address.
// It returns true if the host is "localhost", "127.0.0.1", "::1", or
// if it's an internal private address (e.g., 10.x.x.x, 192.168.x.x).
func isLocalhost(host string) bool {
	// Check for common localhost names and addresses
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Remove port from the host string, if present
	hostWithoutPort, _, err := net.SplitHostPort(host)
	if err != nil {
		// If splitting fails, assume the host string is just the hostname/IP
		hostWithoutPort = host
	}

	// Parse the IP address
	ip := net.ParseIP(hostWithoutPort)
	if ip == nil {
		// If it's not a valid IP, it can't be a localhost IP
		return false
	}

	// Check if the IP is a loopback or private address
	return ip.IsLoopback() || ip.IsPrivate()
}
