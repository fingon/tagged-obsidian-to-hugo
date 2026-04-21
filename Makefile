.PHONY: all
all: check

.PHONY: lint
lint:
	go tool golangci-lint run --fix

.PHONY: test
test:
	go test ./...

.PHONY: build
build:
	go build ./...

.PHONY: check
check: lint test build
