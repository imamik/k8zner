# Implementation Plan - Iteration 1: Foundation & Base Infrastructure

**Goal:** Establish the project structure, implement robust configuration loading, set up the Hetzner Cloud client wrapper, and implement end-to-end testing for provisioning basic networking and firewalls.

**Strategy:** Test-Driven Development (TDD) with an emphasis on "Verification First". We will use a real-world API spike to validate our assumptions about the `hcloud-go` library before codifying them into the application logic.

**Scope:**
1.  **Project Initialization:**
    *   Initialize `go.mod`.
    *   Create directory structure.
    *   Set up linting (golangci-lint) and pre-commit hooks.
2.  **Configuration Module:**
    *   Define configuration structs (Go types) matching the schema in the design doc.
    *   Implement loading from YAML.
    *   Implement validation logic (e.g., required fields, valid regions).
    *   *Test:* Unit tests for config loading and validation.
3.  **API Verification (Spike):**
    *   Create a standalone test `test/e2e/api_verification_test.go`.
    *   Use `HCLOUD_TOKEN` to interact with the real Hetzner API.
    *   **Objective:** Confirm how to idempotently create/update Networks and Firewalls (specifically `SetRules` vs `Update`).
4.  **Hetzner Client Wrapper (Infrastructure Provider):**
    *   Refine `internal/hcloud` package based on spike findings.
    *   Define interfaces for HCloud resources (Network, Firewall, Server, etc.) to allow mocking.
    *   Implement the real HCloud client using `hetznercloud/hcloud-go`.
    *   *Test:* Mock-based unit tests for wrapper methods.
5.  **Base Infrastructure Logic:**
    *   Create `internal/infra` package.
    *   Implement `EnsureNetwork` (idempotent creation of VPC and subnets).
    *   Implement `EnsureFirewall` (idempotent creation of firewall rules).
    *   *Test:* Unit tests with mocks verifying the logic (e.g., "if network exists, don't create").
6.  **CLI Entrypoint:**
    *   Setup `cobra` root command.
    *   Add `apply` command (skeleton).
    *   Wire up config loading and client initialization.
7.  **E2E Testing Harness:**
    *   Create `test/e2e` directory.
    *   Implement a test that:
        1.  Reads `HCLOUD_TOKEN`.
        2.  Generates a random run ID.
        3.  Calls the internal logic to create a Network and Firewall.
        4.  Verifies existence via the real HCloud API (or a separate verify step).
        5.  Cleans up resources.

**Design Principles:**
*   **DRY:** Reuse logic for identifying resources (labels).
*   **SOLID:**
    *   *SRP:* Config loader only loads config. Client wrapper only talks to API.
    *   *DIP:* High-level logic depends on interfaces, not concrete HCloud structs.
*   **Go Best Practices:**
    *   Error handling (wrapping errors).
    *   Context propagation.
    *   Structured logging (slog or zap).

**Deliverables:**
*   A working Go binary that can parse a config file.
*   Code that can provision/reconcile Networks and Firewalls on Hetzner.
*   Passing Unit Tests.
*   Passing E2E Test (runnable via `go test ./test/e2e/...`).
