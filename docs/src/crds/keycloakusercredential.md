# KeycloakUserCredential

Manages user password credentials.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userRef` | ResourceRef | Yes | Reference to KeycloakUser |
| `secretRef` | SecretRef | Yes | Reference to password secret |
| `temporary` | bool | No | Require password change on login |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Credential is set |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: john-password
spec:
  userRef:
    name: john-doe
  secretRef:
    name: john-password-secret
    key: password
  temporary: false
```

## Secret Format

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: john-password-secret
type: Opaque
stringData:
  password: supersecretpassword
```

## Security Notes

- Passwords are stored in Kubernetes Secrets
- The operator only writes passwords to Keycloak, never reads them
- Use RBAC to restrict access to password secrets
- Consider using external secrets management (e.g., HashiCorp Vault)
