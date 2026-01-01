# Implementation Plan - Iteration 1: Foundation, Image Builder & CI/CD

## 1. Overview
This plan details the first iteration of the `hcloud-k8s` refactor. The primary goal is to establish a production-grade project skeleton, implement the configuration subsystem, and deliver the "Image Builder" functionality. This iteration enforces **Test Driven Development (TDD)** and includes a robust CI/CD pipeline to ensure cross-platform compatibility and code quality from day one.

## 2. Goals
1.  **Production-Grade Scaffolding:** complete setup with Makefiles, Linters, and CI/CD pipelines.
2.  **Cross-Platform Compilation:** Automated builds for Linux, macOS, and Windows (amd64/arm64).
3.  **Strict Configuration:** Type-safe configuration parsing with validation.
4.  **Image Builder:** Logic to provision Talos on Hetzner, including snapshot creation.
5.  **Verified Lifecycle:** E2E tests that validate both **creation** and **destruction** of resources to prevent leaks.

## 3. Architecture & Design Principles
*   **TDD First:** Tests are written *before* implementation.
*   **SOLID:**
    *   **Dependency Inversion:** High-level modules (Builder) depend on abstractions (ServerProvisioner), not details (HCloud Client).
    *   **Interface Segregation:** Interfaces are small and specific.
*   **Release Engineering:**
    *   Use `goreleaser` for cross-platform builds.
    *   Use `golangci-lint` for static analysis.

## 4. Implementation Steps

### Phase 1: Project Scaffolding & High-Stakes Setup
**Goal:** robust environment for reliable development.

1.  **Initialization:**
    *   `go mod init github.com/sak-d/hcloud-k8s`
    *   Create standard layout: `cmd/`, `internal/`, `pkg/`, `deploy/` (for CI configs).
2.  **Linter Configuration:**
    *   Add `.golangci.yml` enabling `staticcheck`, `gosec`, `revive`, `errcheck`.
3.  **Build System (Makefile):**
    *   Targets: `fmt`, `lint`, `test`, `build`, `e2e`, `clean`.
4.  **CI/CD (GitHub Actions):**
    *   Workflow `ci.yaml`: Run lint and unit tests on PRs.
    *   Workflow `release.yaml`: Use `goreleaser` to build binaries for Linux/Darwin/Windows (amd64/arm64) on tag push.
5.  **GoReleaser Config:**
    *   Create `.goreleaser.yaml` defining build matrix and binary naming.

### Phase 2: Configuration (TDD)
**Goal:** Type-safe config loading.

1.  **Test (Fail):** Create `internal/config/config_test.go` asserting valid/invalid YAML parsing and validation errors.
2.  **Implement (Pass):**
    *   Define structs with `mapstructure` tags.
    *   Implement `Load()` using `viper` or `gopkg.in/yaml.v3`.
    *   Implement `Validate()` (check for empty tokens, invalid versions).
3.  **Refactor:** Ensure error messages are clear.

### Phase 3: Infrastructure Abstraction (TDD)
**Goal:** Decouple logic from external API.

1.  **Define Interfaces (`internal/hcloud/client.go`):**
    *   `ServerProvisioner`: `CreateServer`, `DeleteServer`.
    *   `SnapshotManager`: `CreateSnapshot`, `FindSnapshot`, `DeleteSnapshot`.
2.  **Test (Mock):** Create `internal/hcloud/mock_client.go` (using `mockery` or manually).
3.  **Implement:** Create `RealClient` wrapper around `hetznercloud/hcloud-go`.

### Phase 4: SSH & Transport (TDD)
**Goal:** Reliable command execution.

1.  **Define Interface:** `Communicator` (`Connect`, `Exec`, `Copy`).
2.  **Test:** Create tests mocking the SSH handshake and command returns.
3.  **Implement:** `internal/ssh` using `golang.org/x/crypto/ssh` with **retry logic** (essential for waiting for rescue mode).

### Phase 5: Image Builder Logic (TDD)
**Goal:** The core business logic.

1.  **Test (Fail):** Create `internal/image/builder_test.go`.
    *   Scenario 1: Snapshot exists -> Skip.
    *   Scenario 2: Success path (Provision -> Wait -> Install -> Snapshot -> Cleanup).
    *   Scenario 3: Error during install -> Cleanup must still trigger.
2.  **Implement (Pass):**
    *   `Builder` struct injecting `ServerProvisioner`, `SnapshotManager`, `Communicator`.
    *   Implement `Build()` method.
    *   **Crucial:** Use `defer` for cleanup to ensure temporary servers are deleted even on panic/error.

### Phase 6: CLI Entrypoint
**Goal:** User interface.

1.  Setup `cmd/hcloud-k8s/main.go` using `cobra`.
2.  Add global flags (`--token`, `--config`, `--dry-run`).
3.  Add `image build` command.
4.  Wire up dependencies (Config -> Client -> Builder).

### Phase 7: E2E Verification & Lifecycle (The "Real World" Test)
**Goal:** Verify creation AND destruction in a real environment.

1.  **Setup:** Create `tests/e2e/main_test.go` with `//go:build e2e`.
2.  **Test Logic (`TestImageLifecycle`):**
    *   **Pre-check:** Ensure clean state (no existing snapshot with test name).
    *   **Execution:** Run `image build`.
    *   **Verification:**
        *   Check Snapshot exists via HCloud API.
        *   Check Metadata (labels correct).
        *   Check Temporary Server is **GONE**.
    *   **Destruction (Test Phase 2):**
        *   Invoke a cleanup function (or future `image destroy` command).
        *   Verify Snapshot is deleted.
3.  **Safety:** The E2E test must have a robust `defer` teardown to delete the snapshot if the test fails mid-way, preventing cost leakage.

## 5. Definition of Done
*   [ ] CI pipeline passes (Lint, Test, Build).
*   [ ] Cross-platform binaries generated.
*   [ ] Unit test coverage > 80%.
*   [ ] E2E test passes with `HCLOUD_TOKEN`, verifying both creation and cleanup.
