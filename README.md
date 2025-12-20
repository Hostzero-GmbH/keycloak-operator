# Keycloak Operator

A Kubernetes operator for managing Keycloak resources declaratively.

## Features

- Manage Keycloak resources as Kubernetes Custom Resources
- Support for realms, clients, users, roles, groups, and more
- Automatic synchronization with Keycloak
- Support for both namespaced and cluster-scoped resources
- Helm chart for easy deployment

## Installation

### Using Helm

```bash
helm repo add keycloak-operator https://hostzero.github.io/keycloak-operator
helm install keycloak-operator keycloak-operator/keycloak-operator
```

### From Source

```bash
git clone https://github.com/hostzero/keycloak-operator.git
cd keycloak-operator
make helm-install
```

## Quick Start

1. Create a secret with Keycloak admin credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-credentials
stringData:
  username: admin
  password: admin
```

2. Create a KeycloakInstance:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
spec:
  baseUrl: http://keycloak:8080
  credentials:
    secretRef:
      name: keycloak-credentials
```

3. Create a KeycloakRealm:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
```

## Supported Resources

| Resource | Description |
|----------|-------------|
| KeycloakInstance | Connection to a Keycloak server |
| KeycloakRealm | Keycloak realm |
| KeycloakClient | OAuth/OIDC client |
| KeycloakUser | User account |
| KeycloakRole | Realm or client role |
| KeycloakGroup | User group |
| KeycloakClientScope | Client scope |
| KeycloakIdentityProvider | External identity provider |
| KeycloakOrganization | Organization (Keycloak 26+) |

## Testing

### Unit Tests

```bash
make test
```

### E2E Tests

Run e2e tests against an existing cluster:

```bash
make test-e2e
```

Run e2e tests in a Kind cluster:

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

## License

Apache License 2.0
