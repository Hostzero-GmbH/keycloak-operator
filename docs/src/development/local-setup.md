# Local Setup

This guide explains how to set up a local development environment.

## Prerequisites

- Go 1.22+
- Docker
- Kind (`brew install kind` or `go install sigs.k8s.io/kind@latest`)
- kubectl
- Helm

## Quick Start with Kind (Recommended)

The easiest way to develop is using Kind:

```bash
# Create cluster and deploy everything
make kind-all
```

This sets up:
- Kind cluster with 3 nodes
- Keycloak instance (admin/admin)
- Operator deployment
- Test resources

### Iterating on Changes

```bash
# After code changes, rebuild and redeploy
make kind-deploy

# Check operator logs
make kind-logs
```

### Accessing Keycloak

To access Keycloak from your local machine:

```bash
# Port-forward Keycloak to localhost:8080
make kind-port-forward
```

Then open http://localhost:8080 (admin/admin).

## Run Against External Keycloak

You can run the operator against any Keycloak instance:

1. Configure kubeconfig for your cluster
2. Install CRDs: `make install`
3. Create a KeycloakInstance pointing to your Keycloak
4. Run locally: `make run`

## Development Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the operator binary |
| `make run` | Run the operator locally |
| `make install` | Install CRDs to cluster |
| `make generate` | Generate DeepCopy methods |
| `make manifests` | Generate CRD manifests |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make lint` | Run golangci-lint |

## IDE Setup

### VS Code

Recommended extensions:
- Go
- YAML
- Kubernetes

Settings (`.vscode/settings.json`):
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.testFlags": ["-v"]
}
```

### GoLand

- Enable Go modules integration
- Configure GOROOT to Go 1.22+
- Set up golangci-lint as external tool

## Debugging

### Local Debugging

1. Set breakpoints in your IDE
2. Run with debug configuration:
   ```bash
   dlv debug ./cmd/main.go
   ```

### Remote Debugging

For debugging in-cluster:

1. Build with debug symbols:
   ```bash
   CGO_ENABLED=0 go build -gcflags="all=-N -l" -o manager ./cmd/main.go
   ```

2. Use `kubectl port-forward` to access debugger port

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig | `~/.kube/config` |
| `KEYCLOAK_URL` | Keycloak URL for tests | `http://localhost:8080` |
| `LOG_LEVEL` | Log level | `info` |
