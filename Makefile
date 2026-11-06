.PHONY: all clean fmt lint lint-version-check test test-cover test-race tidy check verify verify-static verify-template ci bump help

# Go commands
GO    := go
GOFMT := gofmt
GOFILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Pinned golangci-lint release, read from mise.toml — the single source of
# every tool pin: `mise install` provisions it locally and in CI
# (jdx/mise-action), verified against mise.lock. Bump it there in a dedicated
# commit; never edit this line. Compared against `golangci-lint version
# --short`, which prints the bare "MAJOR.MINOR.PATCH".
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# The Go release this module is built with, from go.mod's go line — the only
# Go pin (mise reads the same line). golangci-lint must be built with a Go at
# least this new, or its embedded gofmt and typechecker disagree with the
# toolchain.
GO_VERSION := $(strip $(shell sed -n 's/^go \(.*\)/\1/p' go.mod))
# The major version of the Node on PATH (mise.toml pins it), checked by
# make verify-template. ADR-0003 keeps the template gate free of any package
# install, so the scripts run directly.
NODE_MAJOR := $(shell node -p "process.versions.node.split('.')[0]" 2>/dev/null)

all: fmt lint test

## fmt: Format Go source files
fmt:
	$(GOFMT) -w $(GOFILES)

## lint: Run linter (requires golangci-lint; fails if the mise.toml pin is missing or the installed release differs)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		$(MAKE) --no-print-directory lint-version-check && \
		golangci-lint run; \
	else \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required for make lint (not installed)"; \
		echo "install with: mise install"; \
		exit 1; \
	fi

## lint-version-check: Fail unless the installed golangci-lint is the mise.toml pin and was built with a Go no older than go.mod's go line
lint-version-check:
	@test -n "$(GOLANGCI_LINT_VERSION)" || { echo "mise.toml pins no golangci-lint"; exit 1; }
	@installed="$$(golangci-lint version --short 2>/dev/null)" || { \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required (not installed; run: mise install)"; exit 1; }; \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "expected golangci-lint $(GOLANGCI_LINT_VERSION), found $$installed (run: mise install)"; \
		exit 1; \
	fi; \
	built="$$(golangci-lint version 2>/dev/null | sed -n 's/.*built with go\([0-9.]*\).*/\1/p')"; \
	if [ -n "$$built" ] && [ "$$(printf '%s\n%s\n' "$(GO_VERSION)" "$$built" | sort -V | head -1)" != "$(GO_VERSION)" ]; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) was built with go$$built, older than go.mod's go$(GO_VERSION): bump golangci-lint first"; \
		exit 1; \
	fi

## test: Run tests
test:
	$(GO) test -v ./...

## test-cover: Run tests with coverage
test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## test-race: Run short tests with the race detector
test-race:
	$(GO) test -race -short ./...

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy

## clean: Remove generated artifacts
clean:
	rm -f coverage.out coverage.html
	$(GO) clean

## check: Developer gate (fmt, then the verify gate's static checks, then tests)
check: fmt verify-static test

## verify-static: The non-mutating static checks shared by verify and ci (tidy diff, vet, gofmt -l, exact-pin lint, template gate)
verify-static:
	@echo "==> verify: go.mod is tidy"
	$(GO) mod tidy -diff
	@echo "==> verify: go vet"
	$(GO) vet ./...
	@echo "==> verify: gofmt"
	@out=$$($(GOFMT) -l $(GOFILES)); status=$$?; \
	if [ $$status -ne 0 ]; then \
		echo "gofmt failed (exit $$status); files listed:"; \
		echo "$$out"; \
		exit 1; \
	fi; \
	if [ -n "$$out" ]; then \
		echo "gofmt found unformatted files:"; \
		echo "$$out"; \
		exit 1; \
	fi
	@echo "==> lint (golangci-lint $(GOLANGCI_LINT_VERSION))"
	$(MAKE) --no-print-directory lint-version-check
	golangci-lint run
	@echo "==> verify: template, docs index, risk paths, corrections, vendored copies"
	$(MAKE) --no-print-directory verify-template

## verify-template: Check the agent surfaces, docs index, risk paths, corrections and vendored copies (needs Node.js 22 or later)
verify-template:
	@if ! command -v node >/dev/null 2>&1; then echo "FAIL: verify-template requires Node.js 22 or later (run: mise install)"; exit 1; fi
	@if [ "$(NODE_MAJOR)" = "" ] || [ "$(NODE_MAJOR)" -lt 22 ]; then echo "FAIL: verify-template requires Node.js 22 or later (found: $$(node --version 2>/dev/null || echo missing)); run: mise install"; exit 1; fi
	node scripts/check-template.mjs
	node --test test/*.test.mjs

## verify: Credential-free, non-mutating gate for read-only reviewers (verify-static plus tests)
verify: verify-static
	@echo "==> tests"
	$(GO) test -count=1 ./...

## ci: Run the credential-free CI gate (verify, corrections against BASE when set, then race tests)
ci: verify
	@if [ -n "$(BASE)" ]; then node scripts/check-corrections.mjs --base "$(BASE)"; fi
	@echo "==> race detector"
	$(MAKE) --no-print-directory test-race
	@echo "==> CI gate passed"

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
