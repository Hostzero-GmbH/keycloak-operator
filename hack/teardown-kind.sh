#!/usr/bin/env bash
#
# Teardown the Kind development cluster and clean up resources
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-keycloak-operator-dev}"

echo "Deleting Kind cluster '${CLUSTER_NAME}'..."
kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true

echo "Pruning unused Docker resources..."
docker system prune -f --volumes 2>/dev/null || true

echo "Done!"
