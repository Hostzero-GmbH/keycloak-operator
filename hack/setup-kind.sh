#!/bin/bash
set -e

CLUSTER_NAME="${CLUSTER_NAME:-keycloak-operator-e2e}"

echo "Creating Kind cluster: $CLUSTER_NAME"
kind create cluster --name "$CLUSTER_NAME" --config hack/kind-config.yaml

echo "Deploying Keycloak for testing"
kubectl apply -f hack/keycloak-kind.yaml

echo "Waiting for Keycloak to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/keycloak -n keycloak

echo "Keycloak is ready at http://localhost:8080"
echo "Admin credentials: admin / admin"
