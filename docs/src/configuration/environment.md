# Environment Variables

The operator can be configured using environment variables, which are automatically set when deploying via Helm.

## Operator Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `POD_NAMESPACE` | Namespace where the operator is running | Injected by Kubernetes |
| `POD_NAME` | Name of the operator pod | Injected by Kubernetes |

## Logging

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Log level (debug, info, error) | `info` |
| `LOG_FORMAT` | Log format (json, console) | `json` |

## Metrics

| Variable | Description | Default |
|----------|-------------|---------|
| `METRICS_BIND_ADDRESS` | Address for metrics endpoint | `:8080` |

## Health Probes

| Variable | Description | Default |
|----------|-------------|---------|
| `HEALTH_PROBE_BIND_ADDRESS` | Address for health probes | `:8081` |

## Development

For local development, you can set these in your shell:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=console
make run
```

Or use a `.env` file with your IDE.
