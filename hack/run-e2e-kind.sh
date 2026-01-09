#!/usr/bin/env bash
#
# Run e2e tests against the operator deployed in Kind cluster
#
# This script:
# 1. Ensures the Kind cluster exists with Keycloak
# 2. Builds and deploys the operator
# 3. Runs e2e tests that create actual CRs
#
# Usage:
#   ./hack/run-e2e-kind.sh [test-flags]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-keycloak-operator-dev}"
KEYCLOAK_NAMESPACE="keycloak"
OPERATOR_NAMESPACE="keycloak-operator"
OPERATOR_IMAGE="${IMG:-keycloak-operator:dev}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

check_cluster() {
    if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_error "Kind cluster '${CLUSTER_NAME}' not found."
        log_info "Create it with: make kind-all"
        exit 1
    fi
    kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null 2>&1 || true
}

ensure_keycloak() {
    log_info "Ensuring Keycloak is deployed..."
    
    kubectl create namespace "${KEYCLOAK_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    
    if ! kubectl get deployment keycloak -n "${KEYCLOAK_NAMESPACE}" &>/dev/null; then
        log_info "Deploying Keycloak..."
        
        if ! helm repo list 2>/dev/null | grep -q bitnami; then
            helm repo add bitnami https://charts.bitnami.com/bitnami
        fi
        helm repo update bitnami >/dev/null 2>&1
        
        helm upgrade --install keycloak bitnami/keycloak \
            --namespace "${KEYCLOAK_NAMESPACE}" \
            --set auth.adminUser=admin \
            --set auth.adminPassword=admin \
            --set production=false \
            --wait --timeout 5m
    fi
    
    kubectl wait --for=condition=Ready pod \
        -l app.kubernetes.io/name=keycloak \
        -n "${KEYCLOAK_NAMESPACE}" \
        --timeout=300s
    
    log_success "Keycloak is ready"
}

deploy_operator() {
    log_info "Building and deploying operator..."
    
    cd "${PROJECT_ROOT}"
    
    # Build operator
    make docker-build IMG="${OPERATOR_IMAGE}"
    
    # Load into Kind
    kind load docker-image "${OPERATOR_IMAGE}" --name "${CLUSTER_NAME}"
    
    # Install CRDs
    make install
    
    # Deploy via Helm
    kubectl create namespace "${OPERATOR_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    
    helm upgrade --install keycloak-operator ./charts/keycloak-operator \
        --namespace "${OPERATOR_NAMESPACE}" \
        -f ./charts/keycloak-operator/values-dev.yaml \
        --set image.repository="$(echo ${OPERATOR_IMAGE} | cut -d: -f1)" \
        --set image.tag="$(echo ${OPERATOR_IMAGE} | cut -d: -f2)"
    
    # Wait for operator
    kubectl wait --for=condition=Available deployment/keycloak-operator \
        --namespace "${OPERATOR_NAMESPACE}" \
        --timeout=120s
    
    log_success "Operator deployed"
}

setup_test_resources() {
    log_info "Setting up test resources..."
    
    # Create admin credentials secret for the operator
    kubectl create secret generic keycloak-admin-credentials \
        --namespace "${OPERATOR_NAMESPACE}" \
        --from-literal=username=admin \
        --from-literal=password=admin \
        --dry-run=client -o yaml | kubectl apply -f -
    
    # Get Keycloak service URL (in-cluster)
    local keycloak_url="http://keycloak.${KEYCLOAK_NAMESPACE}.svc.cluster.local"
    
    # Create KeycloakInstance for tests
    cat <<EOF | kubectl apply -f -
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: test-keycloak
  namespace: ${OPERATOR_NAMESPACE}
spec:
  baseUrl: ${keycloak_url}
  credentials:
    secretRef:
      name: keycloak-admin-credentials
EOF

    # Wait for instance to be ready
    log_info "Waiting for KeycloakInstance to be ready..."
    local retries=30
    while [ $retries -gt 0 ]; do
        local ready
        ready=$(kubectl get keycloakinstance test-keycloak -n "${OPERATOR_NAMESPACE}" -o jsonpath='{.status.ready}' 2>/dev/null || echo "false")
        if [ "$ready" = "true" ]; then
            log_success "KeycloakInstance is ready"
            return 0
        fi
        retries=$((retries - 1))
        sleep 2
    done
    
    log_warn "KeycloakInstance may not be ready yet, continuing with tests..."
}

run_e2e_tests() {
    log_info "Running e2e tests..."
    
    cd "${PROJECT_ROOT}"
    
    # Set environment for e2e tests
    export USE_EXISTING_CLUSTER=true
    export KEYCLOAK_INSTANCE_NAME="test-keycloak"
    export KEYCLOAK_INSTANCE_NAMESPACE="${OPERATOR_NAMESPACE}"
    export OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE}"
    
    # Run e2e tests
    go test -v -timeout 30m ./test/e2e/... "$@"
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        log_success "All e2e tests passed!"
    else
        log_error "Some e2e tests failed"
        log_info "Check operator logs: kubectl logs -n ${OPERATOR_NAMESPACE} -l app.kubernetes.io/name=keycloak-operator"
    fi
    
    return $exit_code
}

# Main
check_cluster
ensure_keycloak
deploy_operator
setup_test_resources
run_e2e_tests "$@"
