# Monitoring

The operator exposes Prometheus metrics for monitoring and alerting.

## Metrics Endpoint

Metrics are exposed at `:8080/metrics` by default.

## Available Metrics

### Reconciliation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_reconcile_total` | Counter | controller, result | Total reconciliations |
| `keycloak_operator_reconcile_duration_seconds` | Histogram | controller | Reconciliation duration |

### API Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_api_requests_total` | Counter | method, status | Keycloak API requests |

### Resource Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_managed_resources` | Gauge | kind | Number of managed resources |

## Prometheus Configuration

### ServiceMonitor

If using Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: keycloak-operator
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: keycloak-operator
  endpoints:
    - port: metrics
      interval: 30s
```

### Helm Configuration

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 15s
```

## Grafana Dashboard

Import the provided dashboard for visualization:

```json
{
  "dashboard": {
    "title": "Keycloak Operator",
    "panels": [
      {
        "title": "Reconciliation Rate",
        "targets": [
          {
            "expr": "rate(keycloak_operator_reconcile_total[5m])"
          }
        ]
      }
    ]
  }
}
```

## Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: keycloak-operator
    rules:
      - alert: HighReconcileErrorRate
        expr: |
          rate(keycloak_operator_reconcile_total{result="error"}[5m])
          / rate(keycloak_operator_reconcile_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High reconcile error rate

      - alert: KeycloakAPIErrors
        expr: |
          rate(keycloak_operator_api_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: Keycloak API errors detected
```
