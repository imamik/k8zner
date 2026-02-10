# Proposal: TUI Dashboard for k8zner

## Overview
This proposal outlines the implementation of a "AAA" quality Terminal User Interface (TUI) for the `k8zner` CLI using the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework.

**Goal**: Replace the current scrolling log output with a rich, interactive, real-time dashboard that provides users with better visibility into the provisioning process.

## Motivation
Current `k8zner` provisioning logs are functional but can be overwhelming and hard to scan. A TUI dashboard will improve user experience, enhance professionalism, and clarify the distinct phases of provisioning.

## Proposed Design

The dashboard will be divided into sections corresponding to the provisioning phases:

### Header
- Cluster Name: `production-cluster`
- Region: `fsn1`
- Status: `PROVISIONING` (Animated spinner)

### 1. Infrastructure (Hetzner)
- [x] Network `10.0.0.0/16`
- [x] Firewall Rules
- [x] Load Balancer `1.2.3.4`
- [ ] Placement Group

### 2. Control Plane (Talos)
| Node | IP | Status |
|------|----|--------|
| cp-1 | 10.0.1.2 | ✅ Ready |
| cp-2 | 10.0.1.3 | 🔄 Rebooting... |
| cp-3 | 10.0.1.4 | ⏳ Pending |

### 3. Workers
- Nodes Joined: 0/3
- Configuration Applied: ⏳

### Footer
- Progress Bar: `[██████████░░░░░] 65%`
- Status Message: "Waiting for control plane node 2 to rejoin..."
- Shortcuts: `Ctrl+C` to detach (run in background)

## Technical Implementation

### Dependencies
- **Model**: `charmbracelet/bubbletea` (The ELM architecture)
- **Styling**: `charmbracelet/lipgloss` (CSS for terminal)
- **Forms**: `charmbracelet/huh` (Already in use for `init`, consistent usage)

### Architecture
1. **Model**: Create a `ProvisioningModel` struct in `internal/ui/dashboard` that holds the state of all resources.
2. **Events**: The provisioning pipeline (`internal/provisioning`) currently logs to `stdout`. We will wrap this in a `ui.Reporter` interface that sends `tea.Msg` events to the model instead of (or in addition to) logging.
3. **View**: The `View()` method will render the state using `lipgloss`.

### Phased Rollout
1. **Phase 1**: Implement the UI shell and hook it up to the verified `Apply` command.
2. **Phase 2**: Replace `log.Printf` calls in `provisioning` package with structured event emission.
3. **Phase 3**: Add interactive elements (e.g., viewing detailed logs for a specific step by pressing 'l').

## Usage
The TUI will be the default mode for interactive sessions.
- `k8zner apply` -> Launches TUI.
- `k8zner apply --ci` -> Falls back to standard logging (detected via existing `isatty` check or flag).
