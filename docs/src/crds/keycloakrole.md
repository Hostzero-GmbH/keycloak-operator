# KeycloakRole

> **Identifier field:** Set the role name in the `spec.name` field. It is required and immutable once set. A `name` inside `spec.definition` is tolerated only when it matches `spec.name`; a conflicting value is rejected.

A `KeycloakRole` manages Keycloak roles. Roles can be either realm-level (shared across all clients) or client-level (specific to a single client).

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: my-role
spec:
  # Exactly one of realmRef, clusterRealmRef, or clientRef must be specified
  
  # For realm roles:
  realmRef:
    name: my-realm
  
  # For client roles (the realm comes from the client; do not also set realmRef):
  # clientRef:
  #   name: my-client
  
  # Required: Role definition (Keycloak RoleRepresentation)
  name: admin-role
  definition:
    description: Administrator role
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  roleName: "admin-role"
  roleID: "12345678-1234-1234-1234-123456789abc"
  isClientRole: false
  message: "Role synchronized successfully"
  resourcePath: "/admin/realms/my-realm/roles/admin-role"
  instance:
    instanceRef: my-keycloak
  realm:
    realmRef: my-realm
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
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
  name: admin-role
  definition:
    description: Administrator role with full access
    composite: false
```

### Client Role

The realm is derived from the referenced client, so no `realmRef` is given:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRole
metadata:
  name: my-client-role
  namespace: keycloak
spec:
  clientRef:
    name: my-client
  name: editor
  definition:
    description: Can edit resources
```

## Parent Reference

A `KeycloakRole` can belong to one of three parent types:

| Reference | Scope | Use Case |
|-----------|-------|----------|
| `realmRef` | Realm role | Shared across all clients in the realm |
| `clusterRealmRef` | Realm role | For cluster-scoped realms |
| `clientRef` | Client role | Specific to a single client |

**Note:** Exactly one of these must be specified; setting more than one is rejected.

For a client role, use `clientRef` alone. The realm is taken from the referenced
client, which already belongs to exactly one realm, so `realmRef` and
`clusterRealmRef` must not be combined with `clientRef`.

## Definition Properties

The `definition` field accepts any valid Keycloak [RoleRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#RoleRepresentation):

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Role name (required) |
| `description` | string | Role description |
| `composite` | boolean | Whether this is a composite role |
| `clientRole` | boolean | Whether this is a client role |
| `containerId` | string | Container ID (realm or client ID) |
| `attributes` | object | Custom attributes |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the role is synchronized |
| `status` | string | Current status (e.g., "Ready", "Error") |
| `message` | string | Human-readable status message |
| `resourcePath` | string | Keycloak API path for this role |
| `roleID` | string | Keycloak internal role ID |
| `roleName` | string | The role name in Keycloak |
| `isClientRole` | boolean | Whether this is a client role |
| `clientID` | string | Client ID (for client roles) |
| `instance` | object | Resolved instance reference |
| `realm` | object | Resolved realm reference |
| `observedGeneration` | integer | Last observed generation |
| `conditions` | []Condition | Kubernetes conditions |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcr` | `keycloakroles` |

```bash
kubectl get kcr
```

## Notes

- Role names must be unique within their scope (realm or client)
- When using `clientRef`, the role becomes a client role
- Composite roles can reference other realm or client roles
