# KeycloakInstance

A `KeycloakInstance` represents a connection to a Keycloak server. It serves as the root resource for managing Keycloak configuration.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
spec:
  # Required: Base URL of the Keycloak server
  baseUrl: https://keycloak.example.com
  
  # Optional: Realm to authenticate against (default: master)
  realm: master
  
  # Required: Credentials for admin access
  credentials:
    secretRef:
      # Required: Name of the secret containing credentials
      name: keycloak-admin-credentials
      
      # Optional: Namespace of the secret (defaults to resource namespace)
      namespace: keycloak-operator
      
      # Optional: Key for username (default: username)
      usernameKey: username
      
      # Optional: Key for password (default: password)
      passwordKey: password
```

## Status

```yaml
status:
  # Whether the connection is established
  ready: true
  
  # Keycloak server version
  version: "26.0.0"
  
  # Status message
  message: "Connected successfully"
  
  # Last successful connection time
  lastConnected: "2024-01-01T12:00:00Z"
  
  # Conditions
  conditions:
    - type: Ready
      status: "True"
      reason: Connected
      message: "Successfully connected to Keycloak"
      lastTransitionTime: "2024-01-01T12:00:00Z"
```

## Example

### Basic Instance

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: production-keycloak
  namespace: keycloak-operator
spec:
  baseUrl: https://auth.example.com
  credentials:
    secretRef:
      name: keycloak-admin
```

### With Custom Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: dev-keycloak
spec:
  baseUrl: http://keycloak.keycloak.svc.cluster.local:8080
  realm: admin-realm
  credentials:
    secretRef:
      name: keycloak-credentials
      usernameKey: admin-user
      passwordKey: admin-pass
```

## Credentials Secret

The credentials secret must contain the admin username and password:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-admin-credentials
type: Opaque
stringData:
  username: admin
  password: your-secure-password
```

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kci` | `keycloakinstances` |

```bash
kubectl get kci
```

## Notes

- The operator validates the connection on creation and periodically thereafter
- Connection failures are reflected in the `status.ready` field
- The Keycloak version is detected automatically and stored in `status.version`
