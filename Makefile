TAG=ndajr/urlshortener

.PHONY: lint
lint:
	@golangci-lint run ./... --verbose
	@buf lint

.PHONY: generate
generate:
	@buf generate
	@yq e -P -o=yaml ./cmd/urlshortener-server/apidocs.swagger.json > swagger.yaml

.PHONY: install.deps
install.deps:
	@brew install bufbuild/buf/buf
	@brew install golangci-lint
	@brew install yq

.PHONY: install.tools
install.tools:
	@go install tool

.PHONY: install.cli
install.cli:
	CGO_ENABLED=0 go build -o $(GOPATH)/bin/urlshortener ./cmd/urlshortener-cli

.PHONY: run.dev
run.dev:
	@docker-compose up -d

.PHONY: run
run: run.dev
	@go build -o /tmp/urlshortener ./cmd/urlshortener-server/main.go
	@/tmp/urlshortener

.PHONY: test
test:
	@go test -v -race -count=1 ./internal/systemtest
