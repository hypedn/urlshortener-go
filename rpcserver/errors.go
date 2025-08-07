package rpcserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/status"
)

// httpError is the custom error structure for HTTP responses.
type httpError struct {
	Message string `json:"message"`
}

// NewCustomHTTPErrorHandler creates a custom error handler for the gRPC gateway that marshals
// errors into the httpError struct, omitting the gRPC status code from the response body.
func NewCustomHTTPErrorHandler(logger *slog.Logger) runtime.ErrorHandlerFunc {
	return func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
		st := status.Convert(err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(runtime.HTTPStatusFromCode(st.Code()))

		buf, marshalErr := json.Marshal(httpError{Message: st.Message()})
		if marshalErr != nil {
			logger.Error("failed to marshal http error response body", "error", marshalErr)
			return
		}

		if _, writeErr := w.Write(buf); writeErr != nil {
			logger.Error("failed to write http error response", "error", writeErr)
		}
	}
}
