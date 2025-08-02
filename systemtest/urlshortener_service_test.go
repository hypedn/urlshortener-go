package systemtest

import (
	"context"
	"strings"
	"testing"

	proto "github.com/ndajr/urlshortener-go/rpc/proto/urlshortener/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestURLShortenerService(t *testing.T) {
	ctx := context.Background()

	t.Run("ShortenAndGet", func(t *testing.T) {
		originalURL := "https://github.com/ndajr/urlshortener-go"

		shortenRes, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{OriginalUrl: originalURL})
		require.NoError(t, err)
		require.NotNil(t, shortenRes)
		require.NotEmpty(t, shortenRes.GetShortCode())

		getRes, err := client.GetOriginalURL(ctx, &proto.GetOriginalURLRequest{ShortCode: shortenRes.GetShortCode()})
		require.NoError(t, err)
		require.NotNil(t, getRes)
		require.Equal(t, originalURL, getRes.GetOriginalUrl())
	})

	t.Run("ShortenURL", func(t *testing.T) {
		testCases := []struct {
			name   string
			req    *proto.ShortenURLRequest
			setup  func(t *testing.T)
			assert func(t *testing.T, res *proto.ShortenURLResponse, err error)
		}{
			{
				name: "success",
				req:  &proto.ShortenURLRequest{OriginalUrl: "https://example.com/a/valid/path"},
				assert: func(t *testing.T, res *proto.ShortenURLResponse, err error) {
					require.NoError(t, err)
					require.NotNil(t, res)
					require.NotEmpty(t, res.GetShortCode())
				},
			},
			{
				name: "failure on empty URL",
				req:  &proto.ShortenURLRequest{OriginalUrl: ""},
				assert: func(t *testing.T, res *proto.ShortenURLResponse, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					require.Equal(t, codes.InvalidArgument, st.Code())
				},
			},
			{
				name: "failure on URL without scheme",
				req:  &proto.ShortenURLRequest{OriginalUrl: "google.com"},
				assert: func(t *testing.T, res *proto.ShortenURLResponse, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					require.Equal(t, codes.InvalidArgument, st.Code())
				},
			},
			{
				name: "failure on URL too long",
				req:  &proto.ShortenURLRequest{OriginalUrl: "https://" + strings.Repeat("a", 2084)},
				assert: func(t *testing.T, res *proto.ShortenURLResponse, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					require.Equal(t, codes.InvalidArgument, st.Code())
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.setup != nil {
					tc.setup(t)
				}
				res, err := client.ShortenURL(ctx, tc.req)
				tc.assert(t, res, err)
			})
		}
	})

	t.Run("GetOriginalURL", func(t *testing.T) {
		testCases := []struct {
			name   string
			req    *proto.GetOriginalURLRequest
			assert func(t *testing.T, res *proto.GetOriginalURLResponse, err error)
		}{
			{
				name: "failure on not found",
				req:  &proto.GetOriginalURLRequest{ShortCode: "nonexistent-code"},
				assert: func(t *testing.T, res *proto.GetOriginalURLResponse, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					require.Equal(t, codes.NotFound, st.Code())
				},
			},
			{
				name: "failure on empty short code",
				req:  &proto.GetOriginalURLRequest{ShortCode: ""},
				assert: func(t *testing.T, res *proto.GetOriginalURLResponse, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					require.Equal(t, codes.InvalidArgument, st.Code())
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				res, err := client.GetOriginalURL(ctx, tc.req)
				tc.assert(t, res, err)
			})
		}
	})
}
