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

.PHONY: install
install:
	go install ./cmd/tagged-obsidian-to-hugo

.PHONY: check
check: lint test build
