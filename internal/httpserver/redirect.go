package httpserver

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

		originalURL, err := s.server.GetURL(r.Context(), shortCode)
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				http.NotFound(w, r)
				return
			}

			s.logger.Error("redirectHandler: failed to retrieve URL", "code", shortCode, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, originalURL, http.StatusFound)
	}
}
