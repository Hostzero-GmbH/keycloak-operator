# Keycloak Operator

[![CI](https://github.com/Hostzero-GmbH/keycloak-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/Hostzero-GmbH/keycloak-operator/actions/workflows/ci.yaml)
[![Release](https://github.com/Hostzero-GmbH/keycloak-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/Hostzero-GmbH/keycloak-operator/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Hostzero-GmbH/keycloak-operator)](https://goreportcard.com/report/github.com/Hostzero-GmbH/keycloak-operator)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/hostzero-keycloak-operator)](https://artifacthub.io/packages/search?repo=hostzero-keycloak-operator)

<sub>Sponsored by [Hostzero](https://hostzero.com)</sub>

A Kubernetes operator for managing Keycloak resources declaratively. It uses the `keycloak.hostzero.com/v1beta1` API group.

## Features

- Declarative management of Keycloak resources via Kubernetes CRDs
- Full Keycloak API support via `definition` fields
- Automatic client secret synchronization to Kubernetes Secrets
- Hierarchical resource management (Instance → Realm → Clients/Users)
- Helm chart for easy deployment
- High availability with leader election

## Supported Keycloak Versions

| Keycloak Version | Status |
|------------------|--------|
| 20.x - 26.x | ✅ Supported |
| 19.x and older | ❌ Not supported |

**Minimum supported version: 20.0.0**

The operator validates the Keycloak version on connection and will fail to become ready if an unsupported version is detected. This ensures compatibility with modern Keycloak APIs and security features.

> **Note**: Red Hat Build of Keycloak (RHBK) versions are also supported as they map to upstream Keycloak versions (e.g., RHBK 24.x corresponds to Keycloak 24.x).

## Documentation

📖 **[Read the full documentation](https://keycloak-operator.hostzero.com)**

## Overview

This operator manages Keycloak instances and their resources (realms, clients, users, etc.) as Kubernetes Custom Resources. It provides:

- **KeycloakInstance / ClusterKeycloakInstance**: Connection to a Keycloak server
- **KeycloakRealm / ClusterKeycloakRealm**: Realm configuration
- **KeycloakClient**: OAuth2/OIDC client configuration
- **KeycloakClientScope**: Client scope configuration
- **KeycloakProtocolMapper**: Token claim mappers
- **KeycloakUser**: User management
- **KeycloakUserCredential**: User password management
- **KeycloakGroup**: Group management
- **KeycloakRole**: Realm and client roles
- **KeycloakRoleMapping**: Role-to-user/group assignments
- **KeycloakIdentityProvider**: External identity providers
- **KeycloakComponent**: LDAP federation, key providers
- **KeycloakOrganization**: Organization management (Keycloak 26+)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│  ┌─────────────────┐    ┌──────────────────────────────────┐│
│  │ Keycloak        │    │  Keycloak Operator                ││
│  │ Operator CRDs   │───▶│  ┌────────────────────────────┐  ││
│  │                 │    │  │ Instance Controller        │  ││
│  │ - Instance      │    │  ├────────────────────────────┤  ││
│  │ - Realm         │    │  │ Realm Controller           │  ││
│  │ - Client        │    │  ├────────────────────────────┤  ││
│  │ - User          │    │  │ Client Controller          │  ││
│  │ - ...           │    │  ├────────────────────────────┤  ││
│  └─────────────────┘    │  │ User Controller            │  ││
│                         │  └────────────────────────────┘  ││
│                         └──────────────┬───────────────────┘│
│                                        │                    │
│                                        ▼                    │
│                         ┌──────────────────────────────────┐│
│                         │         Keycloak Server          ││
│                         │         (Admin REST API)         ││
│                         └──────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

```
keycloak-operator/
├── api/
│   └── v1beta1/           # API types (CRDs)
├── cmd/
│   └── main.go            # Operator entrypoint
├── internal/
│   ├── controller/        # Reconciliation logic
│   └── keycloak/          # Keycloak client wrapper
├── config/
│   ├── crd/               # CRD manifests
│   ├── manager/           # Operator deployment
│   ├── rbac/              # RBAC configuration
│   └── samples/           # Example resources
├── test/
│   └── e2e/               # End-to-end tests
├── charts/
│   └── keycloak-operator/ # Helm chart
├── hack/                  # Development scripts
├── Dockerfile
├── Makefile
└── go.mod
```

## Development

### Prerequisites

- Go 1.22+
- Docker
- kubectl
- Kind (`brew install kind`)
- Helm

### Installation

```bash
# Install from OCI registry
helm install keycloak-operator oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### Quick Start (Development)

```bash
# Create Kind cluster with Keycloak and operator deployed
make kind-all

# Check operator logs
make kind-logs

# Apply sample resources
kubectl apply -f config/samples/
```

### Testing

```bash
# Run unit tests (fast, no cluster required)
make test

# Run full E2E tests (requires Kind cluster)
make kind-test
```

## Monitoring

The operator exposes Prometheus metrics at `:8080/metrics` for observability:

- **Reconciliation metrics**: Total reconciliations, duration, errors by controller
- **Resource metrics**: Managed and ready resources by type
- **Keycloak connection**: Connection status, API request counts and latency

Key alerts to configure:
- Connection failures (`keycloak_operator_keycloak_connection_status == 0`)
- High error rate (>10% reconciliation failures)
- Resources not ready for extended periods

See the [Monitoring Documentation](https://keycloak-operator.hostzero.com/monitoring.html) for detailed metrics reference, alerting rules, and Grafana dashboard recommendations.

## API Reference

### KeycloakInstance

Defines a connection to a Keycloak server.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: keycloak-instance
spec:
  baseUrl: http://keycloak:8080
  credentials:
    secretName: keycloak-admin
    usernameKey: username
    passwordKey: password
```

### KeycloakRealm

Defines a realm within a Keycloak instance.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef: keycloak-instance
  definition:
    realm: my-realm
    displayName: My Realm
    enabled: true
```

### KeycloakClient

Defines an OAuth2/OIDC client within a realm.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-client
spec:
  realmRef: my-realm
  definition:
    clientId: my-client
    name: My Application
    publicClient: false
    standardFlowEnabled: true
  clientSecret:
    secretName: my-client-secret
```

## Enterprise Support

This operator is developed and maintained by [**Hostzero GmbH**](https://hostzero.com), a provider of sovereign IT infrastructure solutions.

**For organizations with critical infrastructure needs (KRITIS), we offer:**

- Enterprise support with SLAs
- Security hardening and compliance consulting
- On-premises deployment assistance
- 24/7 incident response
- Training and workshops

[Contact us](https://hostzero.com/contact) for enterprise licensing and support options.

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on our development process and how to submit pull requests.

## Security

For security concerns, please see our [Security Policy](SECURITY.md).

## License

MIT License - see [LICENSE](LICENSE) for details.
