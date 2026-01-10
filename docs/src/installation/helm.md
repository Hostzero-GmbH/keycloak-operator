# Helm Chart Installation

The Keycloak Operator Helm chart provides a flexible way to deploy the operator with customizable settings.

## Installation

### From OCI Registry (Recommended)

```bash
helm install keycloak-operator oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

To install a specific version:

```bash
helm install keycloak-operator oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator \
  --version 0.1.0 \
  --namespace keycloak-operator \
  --create-namespace
```

### From Local Chart

```bash
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### With Custom Values

```bash
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace \
  --values my-values.yaml
```

## Configuration

### Common Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of operator replicas | `1` |
| `image.repository` | Container image repository | `ghcr.io/hostzero-gmbh/keycloak-operator` |
| `image.tag` | Container image tag | Chart appVersion |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Features

| Parameter | Description | Default |
|-----------|-------------|---------|
| `leaderElection.enabled` | Enable leader election | `true` |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.serviceMonitor.enabled` | Create Prometheus ServiceMonitor | `false` |

### CRDs

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crds.install` | Install CRDs with Helm | `true` |
| `crds.keep` | Keep CRDs on uninstall | `true` |

## Example Values Files

### Development

```yaml
# values-dev.yaml
replicaCount: 1
image:
  pullPolicy: Never
  tag: "dev"
resources:
  limits:
    cpu: 200m
    memory: 128Mi
leaderElection:
  enabled: false
logging:
  level: debug
crds:
  keep: false
```

### Production

```yaml
# values-prod.yaml
replicaCount: 2
resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi
metrics:
  serviceMonitor:
    enabled: true
podDisruptionBudget:
  enabled: true
  minAvailable: 1
networkPolicy:
  enabled: true
```

## Upgrading

```bash
helm upgrade keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --values my-values.yaml
```

## Uninstalling

```bash
helm uninstall keycloak-operator --namespace keycloak-operator
```

**Note:** CRDs are kept by default. To remove them:

```bash
kubectl delete crd keycloakinstances.keycloak.hostzero.com
kubectl delete crd keycloakrealms.keycloak.hostzero.com
kubectl delete crd keycloakclients.keycloak.hostzero.com
kubectl delete crd keycloakusers.keycloak.hostzero.com
kubectl delete crd keycloakclientscopes.keycloak.hostzero.com
kubectl delete crd keycloakgroups.keycloak.hostzero.com
kubectl delete crd keycloakidentityproviders.keycloak.hostzero.com
```
