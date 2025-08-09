package systemtest

import (
	"context"
	"strings"
	"testing"

	"github.com/ndajr/urlshortener-go/internal/core"
	proto "github.com/ndajr/urlshortener-go/proto/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestURLShorteningService(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		setup  func(t *testing.T) []core.URL
		assert func(t *testing.T, urls []core.URL)
	}{
		{
			name: "ShortenURL/success",
			assert: func(t *testing.T, _ []core.URL) {
				originalURL := "https://example.com/a/valid/path"
				res, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: originalURL})
				require.NoError(t, err)
				require.NotNil(t, res)
				require.NotEmpty(t, res.GetShortCode())
			},
		},
		{
			name: "ShortenURL/failure_on_empty_url",
			assert: func(t *testing.T, _ []core.URL) {
				_, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: ""})
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "ShortenURL/failure_on_url_without_scheme",
			assert: func(t *testing.T, _ []core.URL) {
				_, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: "google.com"})
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "ShortenURL/failure_on_url_too_long",
			assert: func(t *testing.T, _ []core.URL) {
				originalURL := "https://" + strings.Repeat("a", 2084)
				_, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: originalURL})
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "GetOriginalURL/success",
			setup: func(t *testing.T) []core.URL {
				originalURL := "https://github.com/ndajr/urlshortener-go"
				res, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: originalURL})
				require.NoError(t, err)
				require.NotNil(t, res)
				return []core.URL{{LongURL: originalURL, ShortCode: res.ShortCode}}
			},
			assert: func(t *testing.T, urls []core.URL) {
				for _, url := range urls {
					res, err := client.GetOriginalURL(ctx, &proto.GetOriginalURLRequest{ShortCode: url.ShortCode})
					require.NoError(t, err)
					require.NotNil(t, res)
					require.Equal(t, url.LongURL, res.GetOriginalUrl())
				}
			},
		},
		{
			name: "GetOriginalURL/failure_on_not_found",
			assert: func(t *testing.T, urls []core.URL) {
				_, err := client.GetOriginalURL(ctx, &proto.GetOriginalURLRequest{ShortCode: "nonexistent-code"})
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.NotFound, st.Code())
			},
		},
		{
			name: "GetOriginalURL/failure_on_empty_short_code",
			assert: func(t *testing.T, urls []core.URL) {
				_, err := client.GetOriginalURL(ctx, &proto.GetOriginalURLRequest{ShortCode: ""})
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var urls []core.URL
			if tt.setup != nil {
				urls = tt.setup(t)
			}
			tt.assert(t, urls)
		})
	}
}
