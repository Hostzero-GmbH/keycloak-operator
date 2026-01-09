# KeycloakRole

A `KeycloakRole` manages Keycloak roles. Roles can be either realm-level (shared across all clients) or client-level (specific to a single client).

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: my-role
spec:
  # One of realmRef, clusterRealmRef, or clientRef must be specified
  
  # For realm roles:
  realmRef:
    name: my-realm
  
  # For client roles:
  # clientRef:
  #   name: my-client
  
  # Required: Role definition (Keycloak RoleRepresentation)
  definition:
    name: admin-role
    description: Administrator role
```

## Status

```yaml
status:
  ready: true
  roleName: "admin-role"
  message: "Role synchronized successfully"
```

## Examples

### Realm Role

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: my-realm-role
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    name: admin-role
    description: Administrator role with full access
    composite: false
```

### Client Role

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: my-client-role
  namespace: keycloak
spec:
  clientRef:
    name: my-client
  definition:
    name: editor
    description: Can edit resources
```

## Parent Reference

A `KeycloakRole` can belong to one of three parent types:

| Reference | Scope | Use Case |
|-----------|-------|----------|
| `realmRef` | Realm role | Shared across all clients in the realm |
| `clusterRealmRef` | Realm role | For cluster-scoped realms |
| `clientRef` | Client role | Specific to a single client |

**Note:** Exactly one of these must be specified.

## Definition Properties

The `definition` field accepts any valid Keycloak [RoleRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#RoleRepresentation):

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Role name (required) |
| `description` | string | Role description |
| `composite` | boolean | Whether this is a composite role |
| `composites` | object | Composite role definitions (realm/client roles) |
| `attributes` | object | Custom attributes |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the role is synchronized |
| `status` | string | Current status (e.g., "Ready", "Error") |
| `message` | string | Human-readable status message |
| `roleName` | string | The role name in Keycloak |
| `observedGeneration` | integer | Last observed generation |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcrl` | `keycloakroles` |

```bash
kubectl get kcrl
```

## Notes

- Role names must be unique within their scope (realm or client)
- When using `clientRef`, the role becomes a client role
- Composite roles can reference other realm or client roles
