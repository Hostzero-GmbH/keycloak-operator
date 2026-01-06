# Helm Values

Complete reference for Helm chart configuration.

## Image Configuration

```yaml
image:
  repository: ghcr.io/hostzero/keycloak-operator
  tag: latest
  pullPolicy: IfNotPresent

imagePullSecrets: []
```

## Replicas and Scaling

```yaml
replicaCount: 1

# For HA, use leader election
args:
  - --leader-elect=true
```

## Resources

```yaml
resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

## Operator Arguments

```yaml
args:
  - --max-concurrent-requests=10
  - --leader-elect=false
  - --metrics-bind-address=:8080
  - --health-probe-bind-address=:8081
```

## Service Account

```yaml
serviceAccount:
  create: true
  name: keycloak-operator
  annotations: {}
```

## RBAC

```yaml
rbac:
  create: true
```

## Metrics and Monitoring

```yaml
metrics:
  enabled: true
  port: 8080

  serviceMonitor:
    enabled: false
    namespace: ""
    interval: 30s
    scrapeTimeout: 10s
```

## Node Selection

```yaml
nodeSelector: {}

tolerations: []

affinity: {}
```

## CRDs

```yaml
crds:
  install: true
```

## Complete Example

```yaml
replicaCount: 2

image:
  repository: ghcr.io/hostzero/keycloak-operator
  tag: v1.0.0
  pullPolicy: IfNotPresent

args:
  - --max-concurrent-requests=20
  - --leader-elect=true

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 15s

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: keycloak-operator
          topologyKey: kubernetes.io/hostname
```
