#!/bin/bash
# E2E Upgrade Test Runner
# This script runs the upgrade test in various modes

set -e

# Load environment from .env if exists
if [ -f "../../.env" ]; then
    export $(cat ../../.env | grep -v '^#' | xargs)
fi

if [ -z "$HCLOUD_TOKEN" ]; then
    echo "Error: HCLOUD_TOKEN not set"
    echo "Set it in .env file or export it manually"
    exit 1
fi

# Parse command line arguments
MODE="${1:-full}"

case "$MODE" in
    full)
        echo "Running full E2E upgrade test (all phases)..."
        go test -v -timeout=45m -tags=e2e -run TestE2ELifecycle .
        ;;

    upgrade-only)
        echo "Running upgrade test only (reuses existing cluster)..."
        echo "Note: Requires E2E_CLUSTER_NAME and E2E_KUBECONFIG_PATH"
        E2E_SKIP_SNAPSHOTS=true \
        E2E_SKIP_CLUSTER=true \
        E2E_SKIP_ADDONS=true \
        E2E_SKIP_SCALE=true \
        go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .
        ;;

    quick)
        echo "Running quick test (skip scale and upgrade)..."
        E2E_SKIP_SCALE=true \
        E2E_SKIP_UPGRADE=true \
        go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .
        ;;

    standalone)
        echo "Running standalone upgrade test (fresh cluster)..."
        go test -v -timeout=45m -tags=e2e -run TestE2EUpgradeStandalone .
        ;;

    *)
        echo "Usage: $0 {full|upgrade-only|quick|standalone}"
        echo ""
        echo "Modes:"
        echo "  full          - Run all phases (snapshots, cluster, addons, scale, upgrade)"
        echo "  upgrade-only  - Run only upgrade phase on existing cluster"
        echo "  quick         - Run without scale and upgrade phases (faster)"
        echo "  standalone    - Run dedicated standalone upgrade test"
        echo ""
        echo "Environment variables:"
        echo "  HCLOUD_TOKEN              - Required: Hetzner Cloud API token"
        echo "  E2E_KEEP_SNAPSHOTS       - Optional: Keep snapshots between runs"
        echo "  E2E_CLUSTER_NAME         - Required for upgrade-only: Cluster name"
        echo "  E2E_KUBECONFIG_PATH      - Required for upgrade-only: Path to kubeconfig"
        echo "  E2E_INITIAL_TALOS_VERSION - Optional: Initial Talos version (default: v1.8.2)"
        echo "  E2E_TARGET_TALOS_VERSION  - Optional: Target Talos version (default: v1.8.3)"
        echo "  E2E_INITIAL_K8S_VERSION   - Optional: Initial K8s version (default: v1.30.0)"
        echo "  E2E_TARGET_K8S_VERSION    - Optional: Target K8s version (default: v1.31.0)"
        exit 1
        ;;
esac
