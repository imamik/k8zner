# Implementation Plan - Iteration 1: Foundation & Image Builder

## 1. Overview
This plan details the first iteration of the `hcloud-k8s` refactor. The primary goal is to establish the project skeleton, implement the configuration subsystem, and deliver the "Image Builder" functionality (equivalent to Packer). This iteration will demonstrate the end-to-end flow from CLI execution to Hetzner Cloud resource manipulation, verified by real-world testing.

## 2. Goals
1.  **Project Structure:** Establish a standard Go project layout (`cmd`, `internal`, `pkg`).
2.  **Configuration:** Implement strict configuration parsing and validation.
3.  **Infrastructure Provider:** Create a testable wrapper around the Hetzner Cloud API.
4.  **Image Builder:** Implement the logic to provision a temporary server, install Talos, and create a snapshot.
5.  **Quality Assurance:** Achieve high code coverage via unit tests and verify functionality via an E2E test using `HCLOUD_TOKEN`.

## 3. Architecture & Design Principles
*   **SOLID:**
    *   **Single Responsibility:** Separate packages for `config`, `hcloud` (infra), `image` (business logic), and `ssh` (transport).
    *   **Interface Segregation:** The `ImageBuilder` will depend on a `ServerProvisioner` interface, not the concrete HCloud client, allowing easier mocking.
    *   **Dependency Injection:** Dependencies (Logger, Config, Client) will be injected into commands and services.
*   **DRY:** Common logic (e.g., waiting for SSH, error handling) will be centralized in `internal/utils`.
*   **Error Handling:** Use wrapped errors to provide context (e.g., `fmt.Errorf("failed to provision server: %w", err)`).

## 4. Implementation Steps

### Step 1: Project Initialization
*   Initialize `go.mod` with the module name.
*   Create directory structure:
    ```
    cmd/hcloud-k8s/
    internal/config/
    internal/hcloud/
    internal/image/
    internal/ssh/
    tests/e2e/
    ```

### Step 2: Configuration Subsystem (`internal/config`)
*   Define Go structs mirroring the configuration schema (focusing on `image` and `hcloud` sections for now).
*   Implement `Load(path string)` function using `viper` or standard library.
*   Implement `Validate()` method to ensure required fields (Token, Snapshot Name, Talos Version) are present.
*   **Test:** Unit tests for loading valid/invalid configs.

### Step 3: Infrastructure Provider (`internal/hcloud`)
*   Define an interface `Client` that exposes necessary methods:
    *   `CreateServer(...)`
    *   `DeleteServer(...)`
    *   `CreateSnapshot(...)`
    *   `FindSnapshot(...)`
*   Implement `RealClient` using `github.com/hetznercloud/hcloud-go`.
*   **Test:** Unit tests using a mock implementation of the `Client` interface.

### Step 4: SSH Utilities (`internal/ssh`)
*   Implement a `Client` struct that wraps `golang.org/x/crypto/ssh`.
*   Methods: `Connect`, `RunCommand`, `CopyFile` (or pipe stream).
*   Implement retry logic for connection attempts (waiting for Rescue Mode).

### Step 5: Image Builder Logic (`internal/image`)
*   Create `Builder` struct with dependencies (`Client`, `SSHClient`, `Config`).
*   Implement `Build()` method:
    1.  **Check:** specific snapshot name already exists?
    2.  **Provision:** Create a generic Debian server (lowest cost type).
    3.  **Rescue:** Enable rescue mode and reboot (or create with rescue enabled if supported).
    4.  **Install:**
        *   SSH into server.
        *   Download Talos image (using `wget` on the node or streaming from runner).
        *   `dd` image to `/dev/sda`.
    5.  **Snapshot:** Power off and create a snapshot with labels (version, arch).
    6.  **Cleanup:** Delete the temporary server.
*   **Test:** Unit test `Build` flow using mocks for Cloud and SSH.

### Step 6: CLI Entrypoint (`cmd/hcloud-k8s`)
*   Setup `cobra` root command.
*   Add `image` sub-command with `build` action.
*   Flag parsing (`--config`, `--token`).
*   Wire up dependencies and execute `Builder.Build()`.

### Step 7: End-to-End Test (`tests/e2e`)
*   Create a Go test file protected by build tag `//go:build e2e`.
*   **Test Logic:**
    1.  Load `HCLOUD_TOKEN` from env.
    2.  Construct a test config.
    3.  Run the `ImageBuilder`.
    4.  Verify the snapshot exists in Hetzner (using a separate read-only client).
    5.  Teardown (delete the created snapshot).

## 5. Testing Plan
*   **Unit Tests:** Run with `go test ./...`. Focus on logic and error handling.
*   **E2E Tests:** Run with `HCLOUD_TOKEN=... go test -tags=e2e ./tests/e2e/...`. This ensures the integration with the real Hetzner API works as expected.

## 6. Dependencies
*   `github.com/spf13/cobra` (CLI)
*   `github.com/hetznercloud/hcloud-go/v2/hcloud` (API)
*   `golang.org/x/crypto/ssh` (Remote execution)
*   `gopkg.in/yaml.v3` (Config parsing)
