# Keycloak Operator Helm Chart

Helm chart for deploying the Keycloak Operator.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.0+

## Installation

```bash
helm install keycloak-operator . \
  --namespace keycloak-operator \
  --create-namespace
```

## Configuration

See [values.yaml](values.yaml) for all configuration options.

### Common Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `ghcr.io/hostzero/keycloak-operator` |
| `image.tag` | Image tag | `latest` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `crds.install` | Install CRDs | `true` |

### High Availability

For HA deployments:

```yaml
replicaCount: 2

args:
  - --leader-elect=true
```

### Metrics

Enable Prometheus metrics:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

## Upgrading

```bash
helm upgrade keycloak-operator . \
  --namespace keycloak-operator
```

## Uninstalling

```bash
helm uninstall keycloak-operator --namespace keycloak-operator
```

**Note:** CRDs are not deleted automatically. To remove:

```bash
kubectl delete crds -l app.kubernetes.io/name=keycloak-operator
```

## Values Files

- `values.yaml` - Default values
- `values-dev.yaml` - Development settings
- `values-prod.yaml` - Production settings

## License

Apache License 2.0
