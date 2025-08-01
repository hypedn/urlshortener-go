package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/ndajr/urlshortener-go/datastore"
	proto "github.com/ndajr/urlshortener-go/rpc/proto/urlshortener/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrStoreInternal         = errors.New("internal error")
	ErrStoreDeadlineExceeded = errors.New("the request has timed out, please try again")
	ErrStoreInvalidRequest   = errors.New("invalid request or missing data")
	ErrStoreURLNotFound      = errors.New("url not found")
)

var _ interface {
	proto.URLShortenerServiceServer
} = URLShortenerService{}

// maxURLLength is the maximum allowed length for an original URL, based on common browser limits.
const maxURLLength = 2083

type URLShortenerService struct {
	proto.UnimplementedURLShortenerServiceServer
	db     datastore.Store
	logger *slog.Logger
}

func NewURLShortenerService(db datastore.Store, logger *slog.Logger) URLShortenerService {
	return URLShortenerService{db: db, logger: logger}
}

func (s URLShortenerService) GetOriginalURL(ctx context.Context, req *proto.GetOriginalURLRequest) (*proto.GetOriginalURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "missing short code")
	}
	url, err := s.db.GetURL(ctx, req.ShortCode)
	if err != nil {
		if errors.Is(err, datastore.ErrURLNotFound) {
			return nil, status.Error(codes.NotFound, ErrStoreURLNotFound.Error())
		}
		s.logger.Error("GetOriginalURL internal error", "error", err)
		return nil, status.Error(codes.Internal, ErrStoreInternal.Error())
	}
	return &proto.GetOriginalURLResponse{OriginalUrl: url}, nil
}

func (s URLShortenerService) ShortenURL(ctx context.Context, req *proto.ShortenURLRequest) (*proto.ShortenURLResponse, error) {
	parsedURL, err := parseURL(req.OriginalUrl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, ErrStoreInvalidRequest.Error())
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

	if len(originalURL) > maxURLLength {
		return "", fmt.Errorf("url exceeds maximum length of %d characters", maxURLLength)
	}

	if !strings.HasPrefix(originalURL, "http://") && !strings.HasPrefix(originalURL, "https://") {
		return "", fmt.Errorf("only http and https urls are accepted")
	}

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return "", fmt.Errorf("invalid url format")
	}

	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		return "", fmt.Errorf("invalid url: must be an absolute url with a host")
	}

	return parsedURL.String(), nil
}
