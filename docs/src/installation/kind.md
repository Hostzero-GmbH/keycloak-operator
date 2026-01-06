# Kind Development Setup

Set up a local development environment using Kind.

## Prerequisites

- Docker
- Kind
- kubectl
- Go 1.21+

## Create Kind Cluster

Use the provided script:

```bash
make kind-create
```

This creates a Kind cluster with:
- Keycloak deployed
- Ingress controller configured
- Port mappings for local access

## Manual Setup

If you prefer manual setup:

```bash
# Create cluster
kind create cluster --name keycloak-operator-e2e --config hack/kind-config.yaml

# Deploy Keycloak
kubectl apply -f hack/keycloak-kind.yaml

# Wait for Keycloak
kubectl wait --for=condition=available deployment/keycloak --timeout=300s
```

## Build and Deploy Operator

```bash
# Build the operator image
make docker-build IMG=keycloak-operator:dev

# Load into Kind
kind load docker-image keycloak-operator:dev --name keycloak-operator-e2e

# Deploy with Helm
make helm-install-dev
```

## Access Keycloak

Keycloak is available at:
- URL: http://localhost:8080
- Admin Console: http://localhost:8080/admin
- Username: admin
- Password: admin

## Run Tests

```bash
# Run e2e tests
make test-e2e

# Or run with Kind management
make test-e2e-kind
```

## Cleanup

```bash
make kind-delete
```

## Troubleshooting

### Keycloak not starting

Check pod logs:

```bash
kubectl logs deployment/keycloak
```

### Operator not connecting

Verify the KeycloakInstance status:

```bash
kubectl describe keycloakinstance main
```

### Port conflicts

If port 8080 is in use, modify `hack/kind-config.yaml`:

```yaml
extraPortMappings:
- containerPort: 80
  hostPort: 8081  # Change this
  protocol: TCP
```
