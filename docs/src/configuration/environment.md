# Environment Variables

The operator is configured primarily through command-line flags (set by the Helm chart). A few environment variables are injected or read at runtime.

## Operator Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `POD_NAMESPACE` | Namespace where the operator is running | Injected by Kubernetes |
| `POD_NAME` | Name of the operator pod | Injected by Kubernetes |

## Logging

Logging is configured with zap flags, not environment variables. Via Helm:

```yaml
logging:
  level: info       # debug, info, error
  format: json      # json, console
  development: false
```

Or locally:

```bash
go run ./cmd --zap-log-level=debug --zap-encoder=console --zap-devel
```

## OpenTelemetry

OTLP export is enabled when an endpoint is set. See [Monitoring](../monitoring.md#opentelemetry).

| Variable | Description |
|----------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector endpoint (enables traces and logs) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Override for traces only |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | Override for logs only |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` (default via Helm) or `http/protobuf` |
| `OTEL_SERVICE_NAME` | Defaults to `keycloak-operator` when using the Helm chart |
| `OTEL_TRACES_SAMPLER` | SDK sampler, e.g. `parentbased_traceidratio` |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes |

Any other standard `OTEL_*` variable is passed through to the SDK. Use Helm `extraEnv` for settings not covered by `otel.*`.
