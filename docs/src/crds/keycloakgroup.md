# KeycloakGroup

> **Identifier field:** Set the group name in the `spec.name` field. It is required and immutable once set. A `name` inside `spec.definition` is tolerated only when it matches `spec.name`; a conflicting value is rejected.

A `KeycloakGroup` represents a group within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: my-group
spec:
  # Exactly one of realmRef, clusterRealmRef, or parentGroupRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm (top-level group)
  realmRef:
    name: my-realm
  
  # Option 2: Reference to a ClusterKeycloakRealm (top-level group)
  clusterRealmRef:
    name: my-cluster-realm
  
  # Option 3: Reference to a parent group (nested group; the realm comes from
  # the parent chain, so do not also set realmRef)
  parentGroupRef:
    name: parent-group
  
  # Required: Group definition
  name: my-group
  definition:
    # ... any other properties
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  groupID: "12345678-1234-1234-1234-123456789abc"
  message: "Group synchronized successfully"
  resourcePath: "/admin/realms/my-realm/groups/12345678-..."
  instance:
    instanceRef: my-keycloak
  realm:
    realmRef: my-realm
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
```

## Example

### Basic Group

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: developers
spec:
  realmRef:
    name: my-realm
  name: developers
  definition: {}
```

### Group with Attributes

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: engineering
spec:
  realmRef:
    name: my-realm
  name: engineering
  definition:
    attributes:
      department:
        - Engineering
      cost_center:
        - "1234"
```

### Nested Group

First, create the parent group:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: organization
spec:
  realmRef:
    name: my-realm
  name: organization
  definition: {}
```

Then create child groups. A nested group names only its parent; the realm is
inherited from the parent chain:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: team-alpha
spec:
  parentGroupRef:
    name: organization
  name: team-alpha
  definition: {}
```

## Definition Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Group name (required) |
| `path` | string | Full group path (auto-generated) |
| `attributes` | map | Custom group attributes |
| `realmRoles` | string[] | Realm roles assigned to group |
| `clientRoles` | map | Client roles assigned to group |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcg` | `keycloakgroups` |

```bash
kubectl get kcg
```
