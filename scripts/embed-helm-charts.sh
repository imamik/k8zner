#!/usr/bin/env bash
# embed-helm-charts.sh - Download helm charts for embedding
#
# This script downloads complete helm charts and stores them in
# internal/addons/helm/templates/ for embedding in the Go binary.
#
# Usage: ./scripts/embed-helm-charts.sh [chart-name]
#   chart-name: Optional. If provided, only fetch that chart. Otherwise fetch all.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEMPLATES_DIR="${PROJECT_ROOT}/internal/addons/helm/templates"

# Chart definitions matching terraform variables
declare -A CHARTS=(
    # Format: [chart-name]="repository chart version"
    ["hcloud-ccm"]="https://charts.hetzner.cloud hcloud-cloud-controller-manager 1.29.0"
    ["hcloud-csi"]="https://charts.hetzner.cloud hcloud-csi 2.18.3"
    ["metrics-server"]="https://kubernetes-sigs.github.io/metrics-server/ metrics-server 3.12.2"
    ["cert-manager"]="https://charts.jetstack.io cert-manager v1.19.2"
    ["ingress-nginx"]="https://kubernetes.github.io/ingress-nginx ingress-nginx 4.11.3"
    ["longhorn"]="https://charts.longhorn.io longhorn 1.10.1"
    ["cluster-autoscaler"]="https://kubernetes.github.io/autoscaler cluster-autoscaler-hetzner 1.1.1"
)

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
    exit 1
}

check_dependencies() {
    if ! command -v helm &> /dev/null; then
        error "helm is not installed. Install from https://helm.sh/docs/intro/install/"
    fi
}

download_chart() {
    local chart_name=$1
    local chart_info="${CHARTS[$chart_name]}"

    read -r repository chart version <<< "$chart_info"

    log "Processing $chart_name (version $version)"

    local chart_dir="${TEMPLATES_DIR}/${chart_name}"

    # Remove existing chart directory
    if [ -d "$chart_dir" ]; then
        rm -rf "$chart_dir"
    fi

    # Add repository
    local repo_name="hcloud-k8s-${chart_name}"
    log "  Adding helm repository: $repository"
    helm repo add "$repo_name" "$repository" --force-update >/dev/null 2>&1

    # Pull and extract chart
    log "  Pulling chart $chart version $version"
    helm pull "${repo_name}/${chart}" --version "$version" --untar --untardir "$TEMPLATES_DIR" >/dev/null

    # Rename chart directory to match chart_name
    # Some charts extract to their chart name, not our chart_name
    if [ -d "${TEMPLATES_DIR}/${chart}" ] && [ "${chart}" != "${chart_name}" ]; then
        mv "${TEMPLATES_DIR}/${chart}" "$chart_dir"
    fi

    # Create metadata file
    cat > "${chart_dir}/_chart_info.txt" <<EOF
Chart: ${chart_name}
Version: ${version}
Repository: ${repository}
Downloaded: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
EOF

    log "  ✓ Chart $chart_name embedded successfully"
}

main() {
    check_dependencies

    mkdir -p "$TEMPLATES_DIR"

    if [ $# -gt 0 ]; then
        # Download specific chart
        local chart_name=$1
        if [ -z "${CHARTS[$chart_name]:-}" ]; then
            error "Unknown chart: $chart_name. Available: ${!CHARTS[*]}"
        fi
        download_chart "$chart_name"
    else
        # Download all charts
        log "Embedding all helm charts..."
        for chart_name in "${!CHARTS[@]}"; do
            download_chart "$chart_name"
        done
        log "All charts embedded successfully!"
    fi
}

main "$@"
