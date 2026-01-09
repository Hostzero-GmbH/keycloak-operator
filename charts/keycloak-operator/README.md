# Keycloak Operator Helm Chart

A Helm chart for deploying the Keycloak Operator to Kubernetes.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.0+

## Installation

### Add the repository (if published)

```bash
helm repo add keycloak-operator https://hostzero.github.io/keycloak-operator
helm repo update
```

### Install from local chart

```bash
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### Install with custom values

```bash
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace \
  --values my-values.yaml
```

## Configuration

See [values.yaml](values.yaml) for the full list of configurable parameters.

### Common Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Container image repository | `ghcr.io/hostzero/keycloak-operator` |
| `image.tag` | Container image tag | Chart appVersion |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `leaderElection.enabled` | Enable leader election | `true` |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.serviceMonitor.enabled` | Create ServiceMonitor | `false` |
| `crds.install` | Install CRDs | `true` |
| `crds.keep` | Keep CRDs on uninstall | `true` |

## Usage

### Create a Keycloak Instance Connection

First, create a secret with your Keycloak admin credentials:

```bash
kubectl create secret generic keycloak-credentials \
  --from-literal=username=admin \
  --from-literal=password=your-password
```

Then create a `KeycloakInstance` resource:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-credentials
```

### Create a Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
    displayName: My Realm
```

### Create a Client

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-app
    enabled: true
    protocol: openid-connect
    publicClient: false
  clientSecret:
    secretName: my-app-credentials
    key: clientSecret
```

## Uninstallation

```bash
helm uninstall keycloak-operator -n keycloak-operator
```

**Note:** By default, CRDs are kept when uninstalling. To remove them:

```bash
kubectl delete crd keycloakinstances.keycloak.hostzero.com
kubectl delete crd keycloakrealms.keycloak.hostzero.com
kubectl delete crd keycloakclients.keycloak.hostzero.com
kubectl delete crd keycloakusers.keycloak.hostzero.com
kubectl delete crd keycloakclientscopes.keycloak.hostzero.com
kubectl delete crd keycloakgroups.keycloak.hostzero.com
kubectl delete crd keycloakidentityproviders.keycloak.hostzero.com
```

## Development

### Lint the chart

```bash
helm lint ./charts/keycloak-operator
```

### Template rendering

```bash
helm template keycloak-operator ./charts/keycloak-operator
```

### Package the chart

```bash
helm package ./charts/keycloak-operator
```
