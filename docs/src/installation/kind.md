# Kind Cluster Setup

This guide explains how to set up a local development environment using Kind (Kubernetes in Docker).

## Prerequisites

- Docker
- Kind (`brew install kind` or `go install sigs.k8s.io/kind@latest`)
- kubectl
- Helm

## Quick Setup

The easiest way to get started is using the all-in-one command:

```bash
make kind-all
```

This will:
1. Create a Kind cluster
2. Build the operator image
3. Load the image into Kind
4. Install CRDs
5. Deploy the operator via Helm
6. Deploy Keycloak for testing
7. Create a test KeycloakInstance

## Step-by-Step Setup

### Create the Cluster

```bash
make kind-create
```

This creates a Kind cluster with the following features:
- Multi-node setup (1 control plane + 2 workers)
- Port mappings for Keycloak access (8080, 8443)
- Ingress-ready configuration

### Deploy Keycloak

```bash
make kind-deploy-keycloak
```

Keycloak will be available at:
- **In-cluster**: `http://keycloak.keycloak.svc.cluster.local`
- **External**: `http://localhost:8080` (via NodePort 30080)
- **Credentials**: admin / admin

> **Note**: The NodePort service maps port 30080 to the host's port 8080. If port 8080 is already in use, you can use `make kind-port-forward` as an alternative.

### Deploy the Operator

```bash
make kind-deploy
```

This builds the operator image, loads it into Kind, and deploys via Helm.

## Useful Commands

| Command | Description |
|---------|-------------|
| `make kind-create` | Create the Kind cluster |
| `make kind-delete` | Delete the Kind cluster |
| `make kind-reset` | Delete and recreate the cluster |
| `make kind-status` | Show cluster status |
| `make kind-deploy` | Build and deploy operator |
| `make kind-deploy-keycloak` | Deploy Keycloak |
| `make kind-logs` | Tail operator logs |
| `make kind-port-forward` | Port-forward Keycloak to localhost:8080 |

## Running Tests

Run the full E2E test suite against the Kind cluster:

```bash
make kind-test
```

This sets up port-forwarding automatically and runs all E2E tests.

## Cluster Configuration

The Kind cluster is configured in `hack/kind-config.yaml`:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: keycloak-operator-dev
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      # Keycloak HTTP (NodePort 30080 -> localhost:8080)
      - containerPort: 30080
        hostPort: 8080
        protocol: TCP
      # Keycloak HTTPS
      - containerPort: 30443
        hostPort: 8443
        protocol: TCP
      # Ingress HTTP
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      # Ingress HTTPS
      - containerPort: 443
        hostPort: 443
        protocol: TCP
  - role: worker
  - role: worker
```

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n keycloak-operator -l app.kubernetes.io/name=keycloak-operator -f
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
