.PHONY: fmt lint test build check e2e e2e-fast e2e-snapshot-only e2e-cleanup clean

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

test:
	go test -v ./...

build:
	go build -o bin/hcloud-k8s ./cmd/hcloud-k8s

# Run all checks: format, lint, test, and build
check: fmt lint test build
	@echo "✅ All checks passed!"

# Full e2e test suite (use in CI)
# Builds snapshots, runs all tests, cleans up
e2e:
	go test -v -timeout=1h -tags=e2e ./tests/e2e/...

# Fast e2e for local development
# Keeps snapshots between runs, skips snapshot build test
# First run: ~11-12 min, subsequent runs: ~8 min
e2e-fast:
	@echo "Running fast e2e tests (keeping snapshots, skipping build test)"
	@echo "⚠️  WARNING: This skips TestSnapshotCreation - use 'make e2e' for full validation"
	E2E_KEEP_SNAPSHOTS=true E2E_SKIP_SNAPSHOT_BUILD_TEST=true go test -v -timeout=1h -tags=e2e ./tests/e2e/...

# Test snapshot creation only
# Useful for verifying image builder changes
e2e-snapshot-only:
	go test -v -timeout=30m -tags=e2e -run TestSnapshotCreation ./tests/e2e/...

# Cleanup leftover E2E test resources
# Use this if tests are interrupted or cleanup fails
e2e-cleanup:
	@echo "Cleaning up leftover E2E test resources..."
	@go run tests/e2e/cmd/cleanup/main.go
	@echo "✅ Cleanup complete"

clean:
	rm -rf bin/
