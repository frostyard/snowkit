.PHONY: all clean fmt lint test test-cover tidy check bump help

# Go commands
GO := go
GOFMT := gofmt
GOFILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

all: fmt lint test

## fmt: Format Go source files
fmt:
	$(GOFMT) -w $(GOFILES)

## lint: Run linter
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed, skipping"; exit 0; }
	golangci-lint run

## test: Run tests
test:
	$(GO) test -v ./...

## test-cover: Run tests with coverage
test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy

## clean: Remove generated artifacts
clean:
	rm -f coverage.out coverage.html
	$(GO) clean

## check: Run fmt, lint, and test
check: fmt lint test

## bump: Tag and push next version (requires clean tree and svu)
bump:
	@$(MAKE) check
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working directory not clean. Commit or stash first."; \
		exit 1; \
	fi
	@version=$$(svu next); \
		git tag -a $$version -m "Version $$version"; \
		echo "Tagged $$version"; \
		git push origin $$version

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
