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
  
  # Credentials for admin access
  credentials:
    secretRef:
      name: keycloak-credentials
      namespace: keycloak-operator  # Optional, defaults to resource namespace
      usernameKey: username         # Optional, defaults to "username"
      passwordKey: password         # Optional, defaults to "password"
```

## Resource References

Resources reference their parent using `*Ref` fields:

```yaml
# Realm references an Instance
spec:
  instanceRef:
    name: my-keycloak
    namespace: default  # Optional

# Client references a Realm
spec:
  realmRef:
    name: my-realm
    namespace: default  # Optional
```

## See Also

- [Environment Variables](./configuration/environment.md)
- [Helm Values](./configuration/helm-values.md)
