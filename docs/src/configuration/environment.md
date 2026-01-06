# Environment Variables

Runtime environment configuration.

## Logging

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Log verbosity (debug, info, warn, error) | info |
| `LOG_FORMAT` | Log format (json, console) | json |

## Kubernetes

| Variable | Description | Default |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig file | In-cluster config |
| `WATCH_NAMESPACE` | Namespace to watch (empty = all) | "" |

## Setting Environment Variables

### Via Helm

```yaml
env:
  - name: LOG_LEVEL
    value: debug
  - name: WATCH_NAMESPACE
    value: my-namespace
```

### Via Deployment

```yaml
spec:
  containers:
    - name: manager
      env:
        - name: LOG_LEVEL
          value: debug
```

## Namespace Scoping

By default, the operator watches all namespaces. To restrict to a single namespace:

```yaml
env:
  - name: WATCH_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
```

This configures the operator to only watch its own namespace.
