# Keycloak Operator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hostzero/keycloak-operator)](https://goreportcard.com/report/github.com/hostzero/keycloak-operator)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hostzero/keycloak-operator)](go.mod)

A Kubernetes operator for managing Keycloak resources declaratively.

## Overview

The Keycloak Operator enables GitOps-style management of Keycloak configuration. Define your realms, clients, users, and roles as Kubernetes custom resources, and the operator will synchronize them with your Keycloak instance.

## Features

- **Declarative Configuration**: Manage Keycloak resources as Kubernetes CRDs
- **GitOps Ready**: Store your Keycloak configuration in Git
- **Full Lifecycle Management**: Create, update, and delete resources automatically
- **Multi-Instance Support**: Manage multiple Keycloak instances from a single operator
- **Cluster-Scoped Resources**: Share instances and realms across namespaces
- **Keycloak 26+ Support**: Includes organization management for Keycloak 26+
- **Rate Limiting**: Built-in rate limiting to protect your Keycloak server
- **Prometheus Metrics**: Monitor operator and Keycloak API performance

## Documentation

📖 **[Full Documentation](https://hostzero.github.io/keycloak-operator/)**

## Quick Start

### Install the Operator

```bash
helm install keycloak-operator oci://ghcr.io/hostzero/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### Create a Keycloak Connection

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-credentials
stringData:
  username: admin
  password: admin
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: main
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-credentials
```

### Create a Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app
spec:
  instanceRef:
    name: main
  definition:
    realm: my-app
    enabled: true
```

## Supported Resources

| Resource | Description |
|----------|-------------|
| KeycloakInstance | Connection to a Keycloak server |
| ClusterKeycloakInstance | Cluster-scoped instance connection |
| KeycloakRealm | Keycloak realm |
| ClusterKeycloakRealm | Cluster-scoped realm |
| KeycloakClient | OAuth2/OIDC client |
| KeycloakUser | User account |
| KeycloakRole | Realm or client role |
| KeycloakGroup | User group |
| KeycloakClientScope | Client scope |
| KeycloakRoleMapping | Role assignment to users |
| KeycloakUserCredential | User password |
| KeycloakProtocolMapper | Token mapper |
| KeycloakIdentityProvider | External identity provider |
| KeycloakComponent | Keycloak components (keys, LDAP) |
| KeycloakOrganization | Organization (Keycloak 26+) |

## Installation

### Using Helm

```bash
helm repo add hostzero https://hostzero.github.io/charts
helm install keycloak-operator hostzero/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### From Source

```bash
git clone https://github.com/hostzero/keycloak-operator.git
cd keycloak-operator
make helm-install
```

## Testing

### Unit Tests

```bash
make test
```

### E2E Tests

```bash
# Create Kind cluster with Keycloak
make kind-create

# Run e2e tests
make test-e2e-kind

# Cleanup
make kind-delete
```

## Development

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- Kind (for local testing)
- Helm 3

### Building

```bash
make build
make docker-build IMG=myregistry/keycloak-operator:tag
```

### Documentation

```bash
# Serve docs locally
make docs-serve

# Build docs
make docs
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](docs/src/development/contributing.md) for details.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
