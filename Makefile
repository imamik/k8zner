.PHONY: fmt lint test test-coverage build install check e2e e2e-fast e2e-snapshot-only clean help

# Default target
.DEFAULT_GOAL := help

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

test:
	go test -v -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

build:
	go build -o bin/k8zner ./cmd/k8zner

install: build
	cp bin/k8zner $(GOPATH)/bin/ 2>/dev/null || cp bin/k8zner /usr/local/bin/

# Run all checks: format, lint, test, and build
check: fmt lint test build
	@echo "All checks passed!"

# Full e2e test suite (use in CI)
# Builds snapshots, runs all tests, cleans up
e2e:
	go test -v -timeout=1h -tags=e2e ./tests/e2e/...

# Fast e2e for local development
# Keeps snapshots between runs, skips snapshot build test
e2e-fast:
	@echo "Running fast e2e tests (keeping snapshots, skipping build test)"
	@echo "WARNING: This skips TestSnapshotCreation - use 'make e2e' for full validation"
	E2E_KEEP_SNAPSHOTS=true E2E_SKIP_SNAPSHOT_BUILD_TEST=true go test -v -timeout=1h -tags=e2e ./tests/e2e/...

# Test snapshot creation only
# Useful for verifying image builder changes
e2e-snapshot-only:
	go test -v -timeout=30m -tags=e2e -run TestSnapshotCreation ./tests/e2e/...

clean:
	rm -rf bin/ coverage.out coverage.html

help:
	@echo "k8zner development commands:"
	@echo ""
	@echo "  make build          Build the binary"
	@echo "  make install        Build and install to GOPATH/bin or /usr/local/bin"
	@echo "  make test           Run unit tests with race detection"
	@echo "  make test-coverage  Run tests with coverage report"
	@echo "  make lint           Run golangci-lint"
	@echo "  make fmt            Format code with go fmt"
	@echo "  make check          Run all checks (fmt, lint, test, build)"
	@echo "  make clean          Remove build artifacts"
	@echo ""
	@echo "  make e2e            Run full e2e test suite"
	@echo "  make e2e-fast       Run e2e tests (skip snapshot build)"
	@echo "  make e2e-snapshot-only  Test snapshot creation only"
