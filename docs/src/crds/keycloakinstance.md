# KeycloakInstance

Defines a connection to a Keycloak server.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `baseUrl` | string | Yes | Base URL of the Keycloak server |
| `credentials` | CredentialsSpec | Yes | Admin credentials |

### CredentialsSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secretRef.name` | string | Yes | Name of the secret |
| `secretRef.namespace` | string | No | Namespace of the secret |
| `usernameKey` | string | No | Key for username (default: `username`) |
| `passwordKey` | string | No | Key for password (default: `password`) |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Connection is established |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `version` | string | Keycloak server version |
| `conditions` | []Condition | Standard conditions |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: production
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin-credentials
    usernameKey: admin-user
    passwordKey: admin-pass
```

## Secret Format

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-admin-credentials
type: Opaque
stringData:
  admin-user: admin
  admin-pass: supersecret
```
