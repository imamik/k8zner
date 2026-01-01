# Implementation Plan - Iteration 1: Foundation, Image Builder & CI/CD

## 1. Overview
This plan details the first iteration of the `hcloud-k8s` refactor. The primary goal is to establish a production-grade project skeleton, implement the configuration subsystem, and deliver the "Image Builder" functionality. This iteration enforces **Test Driven Development (TDD)**, **Comprehensive Documentation**, and includes a robust CI/CD pipeline to ensure cross-platform compatibility and code quality from day one.

## 2. Goals
1.  **Production-Grade Scaffolding:** complete setup with Makefiles, Linters, and CI/CD pipelines.
2.  **Cross-Platform Compilation:** Automated builds for Linux, macOS, and Windows (amd64/arm64).
3.  **Strict Configuration:** Type-safe configuration parsing with validation.
4.  **Image Builder:** Logic to provision Talos on Hetzner, including snapshot creation.
5.  **Verified Lifecycle:** E2E tests that validate both **creation** and **destruction** of resources.
6.  **Documentation & Open Source Standards:** High-quality READMEs, GoDoc, and self-documenting code to ensure maintainability and ease of contribution.

## 3. Architecture & Design Principles
*   **Documentation First:**
    *   **GoDoc:** Every exported function, type, and constant must have a clear comment explaining *what* it does and *why*.
    *   **Package READMEs:** Complex packages (`internal/image`, `internal/hcloud`) must have a `README.md` explaining the domain logic and design decisions.
    *   **Clean Code:** Variable names must be descriptive. Comments should explain "why", not "how".
*   **TDD First:** Tests are written *before* implementation.
*   **SOLID:**
    *   **Dependency Inversion:** High-level modules (Builder) depend on abstractions (ServerProvisioner).
    *   **Interface Segregation:** Interfaces are small and specific.
*   **Release Engineering:**
    *   Use `goreleaser` for cross-platform builds.
    *   Use `golangci-lint` for static analysis.

## 4. Implementation Steps

### Phase 1: Project Scaffolding & High-Stakes Setup
**Goal:** robust environment for reliable development.

1.  **Initialization:**
    *   `go mod init github.com/sak-d/hcloud-k8s`
    *   Create standard layout: `cmd/`, `internal/`, `pkg/`, `deploy/`.
    *   **Documentation:** Create a root `README.md` (badges, quick start, architecture overview) and `CONTRIBUTING.md` (guidelines for PRs, issues, coding standards).
2.  **Linter Configuration:**
    *   Add `.golangci.yml` enabling `staticcheck`, `gosec`, `revive`, `errcheck`, `godot` (to check comment formatting).
3.  **Build System (Makefile):**
    *   Targets: `fmt`, `lint`, `test`, `build`, `e2e`, `clean`.
4.  **CI/CD (GitHub Actions):**
    *   Workflow `ci.yaml`: Run lint and unit tests on PRs.
    *   Workflow `release.yaml`: Use `goreleaser` to build binaries.
5.  **GoReleaser Config:**
    *   Create `.goreleaser.yaml`.

### Phase 2: Configuration (TDD)
**Goal:** Type-safe config loading.

1.  **Test (Fail):** Create `internal/config/config_test.go`.
2.  **Implement (Pass):**
    *   Define structs with `mapstructure` tags.
    *   Implement `Load()` and `Validate()`.
3.  **Refactor & Document:** Ensure error messages are clear. Add GoDoc to config structs explaining fields.

### Phase 3: Infrastructure Abstraction (TDD)
**Goal:** Decouple logic from external API.

1.  **Define Interfaces (`internal/hcloud/client.go`):**
    *   `ServerProvisioner`, `SnapshotManager`.
    *   **Doc:** Clearly document the expected behavior of these interfaces (e.g., idempotency requirements).
2.  **Test (Mock):** Create `internal/hcloud/mock_client.go`.
3.  **Implement:** Create `RealClient` wrapper.

### Phase 4: SSH & Transport (TDD)
**Goal:** Reliable command execution.

1.  **Define Interface:** `Communicator`.
2.  **Test:** Mock SSH handshake.
3.  **Implement:** `internal/ssh` with retry logic.
4.  **Doc:** Document the retry strategy (exponential backoff vs fixed).

### Phase 5: Image Builder Logic (TDD)
**Goal:** The core business logic.

1.  **Test (Fail):** Create `internal/image/builder_test.go`.
2.  **Implement (Pass):**
    *   `Builder` struct.
    *   `Build()` method with `defer` cleanup.
3.  **Document:** Create `internal/image/README.md` explaining the server lifecycle state machine and rescue mode nuances.

### Phase 6: CLI Entrypoint
**Goal:** User interface.

1.  Setup `cmd/hcloud-k8s/main.go`.
2.  Add global flags and `image build` command.
3.  **Doc:** Add usage examples to the command help text (`cmd.Long`).

### Phase 7: E2E Verification & Lifecycle
**Goal:** Verify creation AND destruction in a real environment.

1.  **Setup:** Create `tests/e2e/main_test.go`.
2.  **Test Logic:** Verify Build -> Check -> Cleanup flow.
3.  **Doc:** Add comments explaining the `HCLOUD_TOKEN` requirement and safety mechanisms.

## 5. Definition of Done
*   [ ] CI pipeline passes (Lint, Test, Build).
*   [ ] Cross-platform binaries generated.
*   [ ] Unit test coverage > 80%.
*   [ ] E2E test passes with `HCLOUD_TOKEN`.
*   [ ] **Documentation Complete:** All exported symbols documented, READMEs updated, contributing guide present.
