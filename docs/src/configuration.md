# Configuration

The Keycloak Operator can be configured through various mechanisms:

- **Helm Values**: For deployment-time configuration
- **Environment Variables**: For runtime configuration
- **Command-Line Flags**: For operator behavior

## Operator Configuration

The operator accepts the following configuration options:

| Option | Description | Default |
|--------|-------------|---------|
| `--metrics-bind-address` | Address for metrics endpoint | `:8080` |
| `--health-probe-bind-address` | Address for health probes | `:8081` |
| `--leader-elect` | Enable leader election | `false` |

## Keycloak Connection

Each `KeycloakInstance` resource defines how to connect to a Keycloak server:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
spec:
  # Base URL of the Keycloak server
  baseUrl: https://keycloak.example.com

  # Realm to authenticate against (default: master)
  realm: master

  # Authentication: exactly one of auth.passwordGrant or auth.clientCredentials.
  auth:
    passwordGrant:
      secretRef:
        name: keycloak-credentials
        namespace: keycloak-operator  # Optional, defaults to resource namespace
        usernameKey: username         # Optional, defaults to "username"
        passwordKey: password         # Optional, defaults to "password"
```

See [KeycloakInstance](./crds/keycloakinstance.md) for the full auth reference,
including the `clientCredentials` (OAuth2 service-account) variant.

## Resource References

Resources reference their parent using `*Ref` fields. All namespaced references
are **same-namespace only** - every `*Ref` field resolves in the referring
resource's own namespace. For cross-namespace sharing, use the cluster-scoped
`ClusterKeycloakRealm` / `ClusterKeycloakInstance` (via `clusterRealmRef` /
`clusterInstanceRef`).

```yaml
# Realm references an Instance (same namespace)
spec:
  instanceRef:
    name: my-keycloak

# Client references a Realm (same namespace)
spec:
  realmRef:
    name: my-realm
```

## See Also

- [Environment Variables](./configuration/environment.md)
- [Helm Values](./configuration/helm-values.md)
