#!/bin/bash
# Cleanup script for E2E test resources
# Usage: ./cleanup-test.sh [test-id]
#
# If test-id is not provided, it will attempt to find the most recent test
# from the E2E log file.

set -e

# Load token from .env if available
if [ -f .env ]; then
    export HCLOUD_TOKEN=$(grep HCLOUD_TOKEN .env | cut -d= -f2)
fi

if [ -z "$HCLOUD_TOKEN" ]; then
    echo "Error: HCLOUD_TOKEN not set. Please set it in .env or export it."
    exit 1
fi

# Determine test ID
TEST_ID="$1"

if [ -z "$TEST_ID" ]; then
    # Try to extract from most recent e2e log
    if [ -f /tmp/e2e-live.log ]; then
        TEST_ID=$(grep "Starting E2E lifecycle test for cluster:" /tmp/e2e-live.log | tail -1 | sed 's/.*cluster: \(e2e-seq-[0-9]*\).*/\1/')
        if [ -n "$TEST_ID" ]; then
            echo "Detected test-id from log: $TEST_ID"
        fi
    fi
fi

if [ -z "$TEST_ID" ]; then
    echo "Usage: $0 <test-id>"
    echo ""
    echo "Example: $0 e2e-seq-1768340625"
    echo ""
    echo "Or set no argument to auto-detect from /tmp/e2e-live.log"
    exit 1
fi

echo "========================================="
echo "Cleaning up test resources"
echo "========================================="
echo "Test ID: $TEST_ID"
echo ""
echo "This will delete ALL Hetzner Cloud resources with label test-id=$TEST_ID"
read -p "Continue? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

# Build and run cleanup command
echo ""
echo "Building cleanup command..."
~/go/bin/go build -o /tmp/cleanup ./cmd/cleanup

echo ""
echo "Running cleanup..."
/tmp/cleanup -test-id "$TEST_ID"

echo ""
echo "✅ Cleanup complete!"
