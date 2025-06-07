.PHONY: all build fmt lint test
CURRENT_DIR := $(shell pwd)

all: build fmt lint test

build:
	@go build

fmt:
	@gofumpt -l -w $(CURRENT_DIR)

lint:
	@docker run -t --rm -v "$(CURRENT_DIR):/app" -w /app golangci/golangci-lint:v2.1.6 golangci-lint run

test:
	@go test
