#!/usr/bin/env bash
#
# Setup a Kind cluster for local development of the Keycloak Operator
#
# Usage:
#   ./hack/setup-kind.sh [command]
#
# Commands:
#   create    Create the Kind cluster (default)
#   delete    Delete the Kind cluster
#   reset     Delete and recreate the Kind cluster
#   status    Show cluster status
#   load      Load the operator image into the cluster
#   deploy    Deploy the operator to the cluster
#   all       Create cluster, build, load, and deploy operator

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-keycloak-operator-dev}"
KIND_CONFIG="${SCRIPT_DIR}/kind-config.yaml"
KEYCLOAK_MANIFEST="${SCRIPT_DIR}/keycloak-kind.yaml"
OPERATOR_IMAGE="${IMG:-keycloak-operator:dev}"
KEYCLOAK_NAMESPACE="keycloak"
OPERATOR_NAMESPACE="keycloak-operator"
EXPECTED_CONTEXT="kind-${CLUSTER_NAME}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Validate that we're using the correct kubectl context
# This prevents accidentally running commands against production clusters
validate_context() {
    local current_context
    current_context=$(kubectl config current-context 2>/dev/null || echo "")
    
    if [ -z "$current_context" ]; then
        log_error "No kubectl context set"
        return 1
    fi
    
    if [ "$current_context" != "$EXPECTED_CONTEXT" ]; then
        log_error "Wrong kubectl context!"
        echo ""
        echo "  Current context:  $current_context"
        echo "  Expected context: $EXPECTED_CONTEXT"
        echo ""
        echo "This command only runs against the Kind development cluster."
        echo ""
        echo "To fix this, either:"
        echo "  1. Switch context:  kubectl config use-context $EXPECTED_CONTEXT"
        echo "  2. Create cluster:  ./hack/setup-kind.sh create"
        echo ""
        return 1
    fi
    
    return 0
}

# Require correct context - exits on failure
require_kind_context() {
    if ! validate_context; then
        exit 1
    fi
}

check_prerequisites() {
    local missing=()
    
    if ! command -v kind &> /dev/null; then
        missing+=("kind")
    fi
    
    if ! command -v kubectl &> /dev/null; then
        missing+=("kubectl")
    fi
    
    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi
    
    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Install instructions:"
        for tool in "${missing[@]}"; do
            case $tool in
                kind)
                    echo "  kind: brew install kind (or) go install sigs.k8s.io/kind@latest"
                    ;;
                kubectl)
                    echo "  kubectl: brew install kubectl"
                    ;;
                docker)
                    echo "  docker: https://docs.docker.com/get-docker/"
                    ;;
            esac
        done
        exit 1
    fi
    
    # Check if Docker is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker first."
        exit 1
    fi
}

cluster_exists() {
    kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"
}

create_cluster() {
    log_info "Creating Kind cluster '${CLUSTER_NAME}'..."
    
    if cluster_exists; then
        log_warn "Cluster '${CLUSTER_NAME}' already exists"
        return 0
    fi
    
    kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}"
    
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
    
    log_success "Kind cluster '${CLUSTER_NAME}' created successfully"
}

delete_cluster() {
    log_info "Deleting Kind cluster '${CLUSTER_NAME}'..."
    
    if ! cluster_exists; then
        log_warn "Cluster '${CLUSTER_NAME}' does not exist"
        return 0
    fi
    
    kind delete cluster --name "${CLUSTER_NAME}"
    log_success "Kind cluster '${CLUSTER_NAME}' deleted"
}

show_status() {
    if ! cluster_exists; then
        log_warn "Cluster '${CLUSTER_NAME}' does not exist"
        return 1
    fi
    
    require_kind_context
    
    echo ""
    log_info "Cluster: ${CLUSTER_NAME}"
    echo ""
    
    echo "=== Nodes ==="
    kubectl get nodes -o wide
    echo ""
    
    echo "=== Namespaces ==="
    kubectl get namespaces
    echo ""
    
    if kubectl get namespace "${OPERATOR_NAMESPACE}" &> /dev/null; then
        echo "=== Operator Pods ==="
        kubectl get pods -n "${OPERATOR_NAMESPACE}"
        echo ""
    fi
    
    if kubectl get namespace "${KEYCLOAK_NAMESPACE}" &> /dev/null; then
        echo "=== Keycloak Resources ==="
        kubectl get all -n "${KEYCLOAK_NAMESPACE}"
        echo ""
    fi
    
    echo "=== Keycloak CRDs ==="
    kubectl get keycloakinstances,keycloakrealms,keycloakclients -A 2>/dev/null || echo "No Keycloak resources found"
}

build_operator() {
    log_info "Building operator image '${OPERATOR_IMAGE}'..."
    cd "${PROJECT_ROOT}"
    
    make docker-build IMG="${OPERATOR_IMAGE}"
    
    log_success "Operator image built successfully"
}

load_image() {
    log_info "Loading operator image '${OPERATOR_IMAGE}' into Kind cluster..."
    
    if ! cluster_exists; then
        log_error "Cluster '${CLUSTER_NAME}' does not exist. Create it first."
        exit 1
    fi
    
    kind load docker-image "${OPERATOR_IMAGE}" --name "${CLUSTER_NAME}"
    
    log_success "Operator image loaded into cluster"
}

install_crds() {
    require_kind_context
    
    log_info "Installing CRDs..."
    cd "${PROJECT_ROOT}"
    
    make install
    
    log_success "CRDs installed"
}

deploy_operator() {
    require_kind_context
    
    log_info "Deploying operator to cluster..."
    cd "${PROJECT_ROOT}"
    
    # Create namespace if it doesn't exist
    kubectl create namespace "${OPERATOR_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy using Helm with dev values
    helm upgrade --install keycloak-operator ./charts/keycloak-operator \
        --namespace "${OPERATOR_NAMESPACE}" \
        -f ./charts/keycloak-operator/values-dev.yaml \
        --set image.repository="$(echo ${OPERATOR_IMAGE} | cut -d: -f1)" \
        --set image.tag="$(echo ${OPERATOR_IMAGE} | cut -d: -f2)"
    
    log_info "Waiting for operator to be ready..."
    kubectl wait --for=condition=Available deployment/keycloak-operator \
        --namespace "${OPERATOR_NAMESPACE}" \
        --timeout=120s || true
    
    log_success "Operator deployed successfully"
}

deploy_keycloak() {
    require_kind_context
    
    log_info "Deploying Keycloak to cluster for testing..."
    
    # Use manifest-based deployment (no Helm dependency)
    kubectl apply -f "${KEYCLOAK_MANIFEST}"
    
    log_info "Waiting for Keycloak deployment to be available..."
    kubectl wait --for=condition=Available deployment/keycloak \
        --namespace "${KEYCLOAK_NAMESPACE}" \
        --timeout=300s
    
    # Wait for pod to be ready
    kubectl wait --for=condition=Ready pod \
        -l app=keycloak \
        -n "${KEYCLOAK_NAMESPACE}" \
        --timeout=300s
    
    log_success "Keycloak deployed successfully"
    echo ""
    log_info "Keycloak is available at: http://localhost:8080 (via NodePort 30080)"
    log_info "In-cluster URL: http://keycloak.${KEYCLOAK_NAMESPACE}.svc.cluster.local"
    log_info "Admin credentials: admin / admin"
}

deploy_keycloak_helm() {
    require_kind_context
    
    log_info "Deploying Keycloak to cluster using Helm..."
    
    # Create namespace
    kubectl create namespace "${KEYCLOAK_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy Keycloak using Bitnami Helm chart
    if ! helm repo list | grep -q bitnami; then
        helm repo add bitnami https://charts.bitnami.com/bitnami
    fi
    helm repo update bitnami
    
    helm upgrade --install keycloak bitnami/keycloak \
        --namespace "${KEYCLOAK_NAMESPACE}" \
        --set auth.adminUser=admin \
        --set auth.adminPassword=admin \
        --set service.type=NodePort \
        --set service.nodePorts.http=30080 \
        --set production=false \
        --wait --timeout 5m
    
    log_success "Keycloak deployed successfully"
    echo ""
    log_info "Keycloak is available at: http://localhost:8080"
    log_info "Admin credentials: admin / admin"
}

create_test_resources() {
    require_kind_context
    
    log_info "Creating test resources..."
    
    # Create Keycloak admin credentials secret
    kubectl create secret generic keycloak-admin-credentials \
        --namespace "${OPERATOR_NAMESPACE}" \
        --from-literal=username=admin \
        --from-literal=password=admin \
        --dry-run=client -o yaml | kubectl apply -f -
    
    # Create KeycloakInstance
    cat <<EOF | kubectl apply -f -
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: dev-keycloak
  namespace: ${OPERATOR_NAMESPACE}
spec:
  baseUrl: http://keycloak.${KEYCLOAK_NAMESPACE}.svc.cluster.local
  credentials:
    secretRef:
      name: keycloak-admin-credentials
EOF
    
    log_success "Test resources created"
    echo ""
    log_info "Check the KeycloakInstance status:"
    echo "  kubectl get keycloakinstances -n ${OPERATOR_NAMESPACE}"
}

run_all() {
    create_cluster
    build_operator
    load_image
    # CRDs are installed by Helm chart, so skip install_crds
    deploy_operator
    deploy_keycloak
    create_test_resources
    show_status
    
    echo ""
    log_success "Development environment is ready!"
    echo ""
    echo "Next steps:"
    echo "  1. Check operator logs:  kubectl logs -f -n ${OPERATOR_NAMESPACE} -l app.kubernetes.io/name=keycloak-operator"
    echo "  2. Access Keycloak:      http://localhost:8080 (admin/admin)"
    echo "  3. Create a realm:       kubectl apply -f config/samples/keycloak_v1beta1_keycloakrealm.yaml"
    echo ""
}

run_e2e_tests() {
    require_kind_context
    
    log_info "Running e2e tests against Kind cluster..."
    
    cd "${PROJECT_ROOT}"
    export USE_EXISTING_CLUSTER=true
    export KEYCLOAK_INSTANCE_NAME="dev-keycloak"
    export KEYCLOAK_INSTANCE_NAMESPACE="${OPERATOR_NAMESPACE}"
    
    # Set up port-forward to Keycloak so reconciliation tests can connect directly
    log_info "Setting up port-forward to Keycloak..."
    kubectl port-forward -n "${KEYCLOAK_NAMESPACE}" svc/keycloak 8080:80 &>/dev/null &
    local pf_pid=$!
    sleep 2
    
    # Check if port-forward is working
    if kill -0 $pf_pid 2>/dev/null; then
        log_info "Port-forward established (PID: $pf_pid)"
        export KEYCLOAK_URL="http://localhost:8080"
    else
        log_warn "Could not establish port-forward, reconcile tests will be skipped"
    fi
    
    # Run the tests
    go test -v -timeout 30m ./test/e2e/...
    local test_result=$?
    
    # Clean up port-forward
    if kill -0 $pf_pid 2>/dev/null; then
        kill $pf_pid 2>/dev/null || true
        log_info "Port-forward terminated"
    fi
    
    if [ $test_result -eq 0 ]; then
        log_success "E2E tests passed!"
    else
        log_error "E2E tests failed"
    fi
    
    return $test_result
}

print_usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  create            Create the Kind cluster"
    echo "  delete            Delete the Kind cluster"
    echo "  reset             Delete and recreate the Kind cluster"
    echo "  status            Show cluster status"
    echo "  build             Build the operator image"
    echo "  load              Load the operator image into the cluster"
    echo "  install-crds      Install CRDs"
    echo "  deploy            Deploy the operator to the cluster"
    echo "  deploy-keycloak   Deploy Keycloak for testing"
    echo "  test-resources    Create test KeycloakInstance"
    echo "  all               Create cluster, build, load, and deploy everything"
    echo "  test-e2e          Run e2e tests against operator in Kind"
    echo ""
    echo "Environment variables:"
    echo "  KIND_CLUSTER_NAME  Cluster name (default: keycloak-operator-dev)"
    echo "  IMG                Operator image (default: keycloak-operator:dev)"
    echo ""
}

# Main
check_prerequisites

COMMAND="${1:-create}"

case "${COMMAND}" in
    create)
        create_cluster
        ;;
    delete)
        delete_cluster
        ;;
    reset)
        delete_cluster
        create_cluster
        ;;
    status)
        show_status
        ;;
    build)
        build_operator
        ;;
    load)
        load_image
        ;;
    install-crds)
        install_crds
        ;;
    deploy)
        deploy_operator
        ;;
    deploy-keycloak)
        deploy_keycloak
        ;;
    deploy-keycloak-helm)
        deploy_keycloak_helm
        ;;
    test-resources)
        create_test_resources
        ;;
    all)
        run_all
        ;;
    test-e2e)
        run_e2e_tests
        ;;
    help|--help|-h)
        print_usage
        ;;
    *)
        log_error "Unknown command: ${COMMAND}"
        print_usage
        exit 1
        ;;
esac
