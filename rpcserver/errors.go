package rpcserver

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/status"
)

// httpError is the custom error structure for HTTP responses.
type httpError struct {
	Message string `json:"message"`
}

// CustomHTTPErrorHandler is a custom error handler for the gRPC gateway that marshals
// errors into the httpError struct, omitting the gRPC status code from the body.
func CustomHTTPErrorHandler(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
	st := status.Convert(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(runtime.HTTPStatusFromCode(st.Code()))

	buf, _ := json.Marshal(httpError{Message: st.Message()})
	_, err = w.Write(buf)
	if err != nil {
		log.Fatal(err)
	}
}
