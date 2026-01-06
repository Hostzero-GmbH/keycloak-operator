# KeycloakRoleMapping

Assigns roles to a user.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userRef` | ResourceRef | Yes | Reference to KeycloakUser |
| `realmRoles` | []string | No | Realm roles to assign |
| `clientRoles` | map | No | Client roles to assign |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Mapping is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: john-roles
spec:
  userRef:
    name: john-doe
  realmRoles:
    - admin
    - user
  clientRoles:
    my-api:
      - reader
      - writer
    another-client:
      - viewer
```

## Notes

- Roles must exist before creating the mapping
- Removing a role from the spec removes it from the user
- The operator reconciles the exact set of roles specified
