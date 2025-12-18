#!/bin/bash
set -e

CLUSTER_NAME="${CLUSTER_NAME:-keycloak-operator-e2e}"

echo "Building operator image"
make docker-build IMG=keycloak-operator:e2e

echo "Loading image into Kind"
kind load docker-image keycloak-operator:e2e --name "$CLUSTER_NAME"

echo "Installing operator"
make helm-install-dev

echo "Waiting for operator to be ready"
kubectl wait --for=condition=available --timeout=120s deployment/keycloak-operator -n keycloak-operator

echo "Running e2e tests"
USE_EXISTING_CLUSTER=true go test -v ./test/e2e/... -timeout 30m

echo "E2E tests completed"
