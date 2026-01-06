# KeycloakGroup

Manages a user group in a realm.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | Group representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Group is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `groupId` | string | Keycloak group UUID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: developers
spec:
  realmRef:
    name: my-app
  definition:
    name: developers
    attributes:
      team:
        - platform
```

## Nested Groups

```yaml
definition:
  name: engineering
  subGroups:
    - name: frontend
    - name: backend
    - name: devops
```

## Group with Role Mappings

Groups can have default role mappings. Users added to the group automatically receive these roles.

```yaml
definition:
  name: admins
  realmRoles:
    - admin
  clientRoles:
    my-api:
      - admin
```
