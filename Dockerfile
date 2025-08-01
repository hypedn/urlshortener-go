# syntax=docker/dockerfile:1

# ---- Builder Stage ----
# Use a specific version of the Go Alpine image for reproducibility.
# The version must match go.mod
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build tools. git is needed for fetching Go modules if they are not vendored.
RUN apk add --no-cache git

# Copy go.mod and go.sum files to leverage Docker's layer caching.
# This step is only re-run if these files change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the Go application.
# - CGO_ENABLED=0 creates a static binary, which is important for minimal base images.
# - ldflags="-s -w" strips debugging information, reducing the binary size.
# The output is named 'urlshortener' and placed in the root of the builder image.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /urlshortener .

# ---- Final Stage ----
FROM alpine:3.20

# Install ca-certificates for any potential TLS/SSL connections (e.g., to a database).
RUN apk --no-cache add ca-certificates

# Create a non-root user and group for security.
# Running as a non-root user is a security best practice.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=builder /urlshortener .
COPY .migrations ./.migrations
RUN chown -R appuser:appgroup /app
USER appuser
EXPOSE 8080 8081
ENTRYPOINT ["/app/urlshortener"]
