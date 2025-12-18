#!/bin/bash
set -e

CLUSTER_NAME="${CLUSTER_NAME:-keycloak-operator-e2e}"

echo "Deleting Kind cluster: $CLUSTER_NAME"
kind delete cluster --name "$CLUSTER_NAME"

echo "Cluster deleted"
