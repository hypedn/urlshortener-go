package httpserver

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "github.com/ndajr/urlshortener-go/rpcserver/proto/urlshortener/v1"
)

func (s *Server) redirectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the short code from the request URL path.
		// e.g., for a request to "/aBcDeF1", this will be "aBcDeF1".
		shortCode := strings.TrimPrefix(r.URL.Path, "/")

		// If the path is empty, it means the request was for the root "/".
		// We redirect to the API documentation page.
		if shortCode == "" {
			http.Redirect(w, r, docsURL, http.StatusFound)
			return
		}

		// Call the gRPC service to get the long URL. This keeps the HTTP layer
		// decoupled from the data layer, using the gRPC API as the boundary.
		resp, err := s.server.URLShorteningService.GetOriginalURL(r.Context(), &proto.GetOriginalURLRequest{ShortCode: shortCode})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				http.NotFound(w, r)
				return
			}

			s.logger.Error("redirectHandler: failed to retrieve URL", "code", shortCode, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Issue a 302 Found redirect.
		http.Redirect(w, r, resp.GetOriginalUrl(), http.StatusFound)
	}
}
