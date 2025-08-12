# ---- Build Stage ----
# The version must match go.mod
FROM golang:1.24-alpine AS builder

ARG APP_VERSION="v0.0.0-dev"
ARG GIT_COMMIT="dev"

# Install build dependencies (git is needed for go modules)
RUN apk add --no-cache git
WORKDIR /src
# Copy go module files and download dependencies to leverage Docker's layer cache
COPY go.mod go.sum ./
RUN go mod download
# Copy the rest of the source code
COPY . .
# Build the application binary. CGO_ENABLED=0 creates a static binary.
# The -ldflags option is used to embed build-time variables into the binary.
# -w -s strips debug information, reducing the binary size.
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s -X main.version=${APP_VERSION} -X main.gitCommit=${GIT_COMMIT}" \
    -o /app/urlshortener \
    ./cmd/urlshortener-server

# ---- Final Stage ----
FROM alpine:3.20

RUN apk --no-cache add ca-certificates
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/urlshortener .
COPY .migrations ./.migrations
RUN chown -R appuser:appgroup /app
USER appuser
EXPOSE 8080 8081
ENTRYPOINT ["/app/urlshortener"]
