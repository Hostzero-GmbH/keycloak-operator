# Installation

This section covers various ways to install the Keycloak Operator.

## Prerequisites

- Kubernetes cluster (1.25+)
- kubectl configured to access your cluster
- Helm 3 (for Helm installation)

## Installation Methods

Choose the installation method that best fits your needs:

- [Quick Start](./installation/quick-start.md) - Get up and running in minutes
- [Helm Installation](./installation/helm.md) - Production-ready Helm chart
- [Kind Development Setup](./installation/kind.md) - Local development with Kind

## Verifying Installation

After installation, verify the operator is running:

```bash
kubectl get pods -n keycloak-operator
```

You should see the operator pod in Running state:

```
NAME                                 READY   STATUS    RESTARTS   AGE
keycloak-operator-7d8f9b6c4-x2j5k   1/1     Running   0          1m
```

Check that CRDs are installed:

```bash
kubectl get crds | grep keycloak
```

Expected output:

```
clusterkeycloakinstances.keycloak.hostzero.com   2024-01-01T00:00:00Z
clusterkeycloakrealms.keycloak.hostzero.com      2024-01-01T00:00:00Z
keycloakclients.keycloak.hostzero.com            2024-01-01T00:00:00Z
keycloakclientscopes.keycloak.hostzero.com       2024-01-01T00:00:00Z
keycloakcomponents.keycloak.hostzero.com         2024-01-01T00:00:00Z
keycloakgroups.keycloak.hostzero.com             2024-01-01T00:00:00Z
keycloakidentityproviders.keycloak.hostzero.com  2024-01-01T00:00:00Z
keycloakinstances.keycloak.hostzero.com          2024-01-01T00:00:00Z
keycloakorganizations.keycloak.hostzero.com      2024-01-01T00:00:00Z
keycloakprotocolmappers.keycloak.hostzero.com    2024-01-01T00:00:00Z
keycloakrealms.keycloak.hostzero.com             2024-01-01T00:00:00Z
keycloakrolemappings.keycloak.hostzero.com       2024-01-01T00:00:00Z
keycloakroles.keycloak.hostzero.com              2024-01-01T00:00:00Z
keycloakusercredentials.keycloak.hostzero.com    2024-01-01T00:00:00Z
keycloakusers.keycloak.hostzero.com              2024-01-01T00:00:00Z
```
