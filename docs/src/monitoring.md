# Monitoring

The Keycloak Operator exposes Prometheus metrics to enable comprehensive monitoring and alerting for your Keycloak resources.

## Metrics Endpoint

Metrics are exposed at `:8080/metrics` by default (configurable via `--metrics-bind-address`).

## Available Metrics

### Reconciliation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_reconcile_total` | Counter | `controller`, `result` | Total number of reconciliations per controller |
| `keycloak_operator_reconcile_duration_seconds` | Histogram | `controller` | Time spent in reconciliation |
| `keycloak_operator_reconcile_errors_total` | Counter | `controller`, `error_type` | Total errors by controller and type |
| `keycloak_operator_last_reconcile_timestamp_seconds` | Gauge | `controller` | Timestamp of last successful reconciliation |

### Resource Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_resources_managed` | Gauge | `resource_type`, `namespace` | Number of managed resources |
| `keycloak_operator_resources_ready` | Gauge | `resource_type`, `namespace` | Number of resources in ready state |

### Keycloak Connection Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_keycloak_connection_status` | Gauge | `instance`, `namespace` | Connection status (1=connected, 0=disconnected) |
| `keycloak_operator_keycloak_api_requests_total` | Counter | `instance`, `method`, `endpoint`, `status` | Total Keycloak API requests |
| `keycloak_operator_keycloak_api_latency_seconds` | Histogram | `instance`, `method`, `endpoint` | Keycloak API latency |

### Controller Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keycloak_operator_workqueue_depth` | Gauge | `controller` | Work queue depth per controller |

## Error Types

The `error_type` label can have the following values:

- `fetch_error` - Failed to fetch the Kubernetes resource
- `connection_error` - Failed to connect to Keycloak
- `instance_not_ready` - Referenced KeycloakInstance is not ready
- `realm_not_ready` - Referenced KeycloakRealm is not ready
- `invalid_definition` - Invalid resource definition (JSON parsing failed)
- `keycloak_api_error` - Keycloak API call failed
- `secret_sync_error` - Failed to synchronize client secret

## Monitoring Recommendations

### Critical Alerts

Set up alerts for these critical conditions:

#### 1. Keycloak Connection Failures

```yaml
alert: KeycloakConnectionDown
expr: keycloak_operator_keycloak_connection_status == 0
for: 5m
labels:
  severity: critical
annotations:
  summary: "Keycloak connection lost"
  description: "Instance {{ $labels.instance }} in {{ $labels.namespace }} has been disconnected for 5 minutes"
```

#### 2. High Reconciliation Error Rate

```yaml
alert: KeycloakOperatorHighErrorRate
expr: |
  rate(keycloak_operator_reconcile_errors_total[5m]) 
  / rate(keycloak_operator_reconcile_total[5m]) > 0.1
for: 10m
labels:
  severity: warning
annotations:
  summary: "High reconciliation error rate"
  description: "Controller {{ $labels.controller }} has >10% error rate"
```

#### 3. Resources Not Ready

```yaml
alert: KeycloakResourcesNotReady
expr: |
  keycloak_operator_resources_managed - keycloak_operator_resources_ready > 0
for: 15m
labels:
  severity: warning
annotations:
  summary: "Keycloak resources not ready"
  description: "{{ $value }} {{ $labels.resource_type }} resources are not ready in {{ $labels.namespace }}"
```

#### 4. Slow Reconciliation

```yaml
alert: KeycloakSlowReconciliation
expr: |
  histogram_quantile(0.99, 
    rate(keycloak_operator_reconcile_duration_seconds_bucket[5m])
  ) > 30
for: 10m
labels:
  severity: warning
annotations:
  summary: "Slow reconciliation detected"
  description: "Controller {{ $labels.controller }} p99 reconciliation time exceeds 30s"
```

#### 5. Controller Stale

```yaml
alert: KeycloakControllerStale
expr: |
  time() - keycloak_operator_last_reconcile_timestamp_seconds > 600
for: 5m
labels:
  severity: critical
annotations:
  summary: "Controller not reconciling"
  description: "Controller {{ $labels.controller }} has not reconciled for 10+ minutes"
```

### Dashboard Recommendations

Create a Grafana dashboard with these panels:

1. **Overview**
   - Total managed resources by type
   - Ready vs non-ready resources
   - Keycloak instance connection status

2. **Reconciliation Performance**
   - Reconciliation rate per controller
   - Reconciliation duration (p50, p95, p99)
   - Error rate over time

3. **Errors & Issues**
   - Error breakdown by type
   - Recent error spikes
   - Connection failures over time

4. **Keycloak API**
   - API request rate by endpoint
   - API latency distribution
   - Error responses by status code

### Key Metrics to Watch

| Metric | Normal Range | Action if Abnormal |
|--------|--------------|-------------------|
| Connection status | 1 | Check Keycloak availability, credentials |
| Error rate | < 5% | Review logs, check Keycloak health |
| Reconcile duration p99 | < 10s | Check Keycloak performance |
| Queue depth | < 50 | Scale operator or reduce resources |
| Resources not ready | 0 | Check individual resource status |

## Prometheus ServiceMonitor

If using Prometheus Operator, create a ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: keycloak-operator
  labels:
    app: keycloak-operator
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

## Helm Chart Configuration

Enable metrics in the Helm chart:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s
    labels: {}
```
