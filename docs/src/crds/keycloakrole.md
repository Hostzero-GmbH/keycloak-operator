# KeycloakRole

Manages a realm or client role.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `clientRef` | ResourceRef | No | Reference to KeycloakClient (for client roles) |
| `definition` | object | Yes | Role representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Role is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `roleId` | string | Keycloak role UUID |

## Realm Role Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: admin-role
spec:
  realmRef:
    name: my-app
  definition:
    name: admin
    description: Administrator role with full access
```

## Client Role Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: api-reader
spec:
  realmRef:
    name: my-app
  clientRef:
    name: my-api
  definition:
    name: reader
    description: Read-only access to API
```

## Composite Roles

```yaml
definition:
  name: super-admin
  composite: true
  composites:
    realm:
      - admin
      - user
    client:
      my-api:
        - reader
        - writer
```
