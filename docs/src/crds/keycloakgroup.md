# KeycloakGroup

A `KeycloakGroup` represents a group within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: my-group
spec:
  # One of realmRef or clusterRealmRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm
    namespace: default  # Optional, defaults to same namespace
  
  # Option 2: Reference to a ClusterKeycloakRealm
  clusterRealmRef:
    name: my-cluster-realm
  
  # Optional: Reference to parent group (for nested groups)
  parentGroupRef:
    name: parent-group
    namespace: default  # Optional, defaults to same namespace
  
  # Required: Group definition
  definition:
    name: my-group
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
  definition:
    name: developers
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
  definition:
    name: engineering
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
  definition:
    name: organization
```

Then create child groups:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakGroup
metadata:
  name: team-alpha
spec:
  realmRef:
    name: my-realm
  parentGroupRef:
    name: organization
  definition:
    name: team-alpha
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
