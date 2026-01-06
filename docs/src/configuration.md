# Configuration

This section covers operator configuration options.

## Configuration Methods

The operator can be configured through:

1. **Helm Values** - Primary configuration method for Kubernetes deployments
2. **Command-line Arguments** - Direct flags to the operator binary
3. **Environment Variables** - Runtime configuration

## Quick Reference

| Setting | Helm Value | CLI Flag | Default |
|---------|------------|----------|---------|
| Max concurrent requests | `args` | `--max-concurrent-requests` | 10 |
| Leader election | `args` | `--leader-elect` | false |
| Metrics address | `args` | `--metrics-bind-address` | :8080 |
| Health probe address | `args` | `--health-probe-bind-address` | :8081 |

## Sections

- [Helm Values](./configuration/helm-values.md) - Complete Helm chart configuration
- [Environment Variables](./configuration/environment.md) - Runtime environment options
