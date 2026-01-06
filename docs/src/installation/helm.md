# Helm Installation

Production-ready installation using Helm.

## Add the Helm Repository

```bash
helm repo add hostzero https://hostzero.github.io/charts
helm repo update
```

## Install with Default Values

```bash
helm install keycloak-operator hostzero/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

## Install with Custom Values

Create a `values.yaml` file:

```yaml
replicaCount: 2

image:
  repository: ghcr.io/hostzero/keycloak-operator
  tag: latest
  pullPolicy: IfNotPresent

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

args:
  - --max-concurrent-requests=20
  - --leader-elect=true

serviceAccount:
  create: true
  name: keycloak-operator

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

Install with custom values:

```bash
helm install keycloak-operator hostzero/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace \
  -f values.yaml
```

## Upgrading

```bash
helm upgrade keycloak-operator hostzero/keycloak-operator \
  --namespace keycloak-operator \
  -f values.yaml
```

## Uninstalling

```bash
helm uninstall keycloak-operator --namespace keycloak-operator
```

Note: CRDs are not removed automatically. To remove them:

```bash
kubectl delete crds -l app.kubernetes.io/name=keycloak-operator
```

## Configuration Reference

See [Helm Values](../configuration/helm-values.md) for all available options.
