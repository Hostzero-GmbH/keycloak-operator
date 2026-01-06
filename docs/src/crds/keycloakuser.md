# KeycloakUser

Manages a user account in a realm.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | User representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | User is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `userId` | string | Keycloak user UUID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: john-doe
spec:
  realmRef:
    name: my-app
  definition:
    username: johndoe
    email: john@example.com
    firstName: John
    lastName: Doe
    enabled: true
    emailVerified: true
    attributes:
      department:
        - engineering
```

## Setting Password

Use [KeycloakUserCredential](./keycloakusercredential.md) to set user passwords.

## Assigning Roles

Use [KeycloakRoleMapping](./keycloakrolemapping.md) to assign roles to users.

## Definition Fields

| Field | Type | Description |
|-------|------|-------------|
| `username` | string | Username (required) |
| `email` | string | Email address |
| `firstName` | string | First name |
| `lastName` | string | Last name |
| `enabled` | bool | Whether user is enabled |
| `emailVerified` | bool | Email verification status |
| `attributes` | map | Custom attributes |
| `groups` | []string | Group memberships |
