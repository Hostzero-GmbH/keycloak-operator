# Helm Values Reference

Complete reference for all Helm chart values.

## Global

```yaml
# Number of replicas
replicaCount: 1

# Image configuration
image:
  repository: ghcr.io/hostzero/keycloak-operator
  pullPolicy: IfNotPresent
  tag: ""  # Defaults to Chart.appVersion

# Image pull secrets
imagePullSecrets: []

# Name overrides
nameOverride: ""
fullnameOverride: ""
```

## Service Account

```yaml
serviceAccount:
  create: true
  annotations: {}
  name: ""
```

## Pod Configuration

```yaml
# Pod annotations
podAnnotations: {}

# Pod labels
podLabels: {}

# Pod security context
podSecurityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault

# Container security context
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
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

## Scheduling

```yaml
nodeSelector: {}
tolerations: []
affinity: {}
priorityClassName: ""
```

## Leader Election

```yaml
leaderElection:
  enabled: true
```

## Metrics

```yaml
metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: false
    additionalLabels: {}
    interval: 30s
    scrapeTimeout: 10s
```

## Health Probes

```yaml
health:
  port: 8081
```

## Logging

```yaml
logging:
  level: info      # debug, info, error
  format: json     # json, console
  development: false
```

## Performance Tuning

```yaml
performance:
  # Sync period for re-checking successfully reconciled resources
  # Higher values reduce Keycloak API load but increase drift detection time
  syncPeriod: "5m"        # e.g., "5m", "30m", "1h"
  
  # Maximum concurrent requests to Keycloak (0 = no limit)
  # Lower values reduce Keycloak load but slow reconciliation
  maxConcurrentRequests: 10
```

For large deployments (100+ resources), consider:
```yaml
performance:
  syncPeriod: "30m"
  maxConcurrentRequests: 5
```

## RBAC

```yaml
rbac:
  create: true
```

## CRDs

```yaml
crds:
  install: true
  keep: true  # Keep CRDs on uninstall
```

## Extra Configuration

```yaml
# Additional environment variables
extraEnv: []
  # - name: MY_VAR
  #   value: my-value

# Additional volumes
extraVolumes: []

# Additional volume mounts
extraVolumeMounts: []
```

## High Availability

```yaml
# Termination grace period
terminationGracePeriodSeconds: 10

# Network policy
networkPolicy:
  enabled: false
  ingress: []
  egress: []

# Pod disruption budget
podDisruptionBudget:
  enabled: false
  minAvailable: 1
  maxUnavailable: ""
```
