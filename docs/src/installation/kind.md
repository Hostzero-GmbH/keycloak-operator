# Kind Cluster Setup

This guide explains how to set up a local development environment using Kind (Kubernetes in Docker).

## Prerequisites

- Docker
- Kind (`brew install kind` or `go install sigs.k8s.io/kind@latest`)
- kubectl
- Helm

## Quick Setup

```bash
make kind-all
```

This creates a Kind cluster and deploys everything:
- Kind cluster with 3 nodes
- Keycloak instance (admin/admin at localhost:8080)
- Operator deployment
- Test KeycloakInstance resource

## Development Workflow

```bash
# 1. Initial setup (once)
make kind-all

# 2. Start port-forward in a separate terminal
make kind-port-forward

# 3. After code changes, rebuild and restart
make kind-redeploy

# 4. Run tests
make kind-test-run

# 5. Run specific test
make kind-test-run TEST_RUN=TestMyFeature
```

## Commands

| Command | Description |
|---------|-------------|
| `make kind-all` | Full setup: cluster + Keycloak + operator |
| `make kind-redeploy` | Rebuild and restart operator (fast iteration) |
| `make kind-test-run` | Run e2e tests (use `TEST_RUN=TestName` to filter) |
| `make kind-logs` | Tail operator logs |
| `make kind-port-forward` | Port-forward Keycloak to localhost:8080 |
| `make kind-reset` | Reset cluster to clean state |
| `make kind-delete` | Delete the Kind cluster |

## Troubleshooting

### Check Operator Logs

```bash
make kind-logs
```

### Check Keycloak Logs

```bash
kubectl logs -n keycloak -l app=keycloak -f
```

### Verify CRDs

```bash
kubectl get crds | grep keycloak
```

### Check Resource Status

```bash
kubectl get keycloakinstances,keycloakrealms,keycloakclients -A
```
