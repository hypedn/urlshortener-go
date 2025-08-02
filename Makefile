.PHONY: api
lint:
	@golangci-lint run ./...
	@buf lint

.PHONY: generate
generate:
	@buf generate

.PHONY: install.buf
install.buf:
	@brew install bufbuild/buf/buf

.PHONY: install.deps
install.deps:
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest

.PHONY: run.dev
run.dev:
	@docker-compose up -d

.PHONY: run.cli
run.cli:
	@go run ./cmd/urlshortener/main.go

.PHONY: run
run: run.dev
	@go run main.go

.PHONY: test
test:
	@go test -v -race -count=1 ./systemtest
