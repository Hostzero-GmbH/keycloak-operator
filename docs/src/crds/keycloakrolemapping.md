# KeycloakRoleMapping

The `KeycloakRoleMapping` resource assigns Keycloak roles to users or groups.

## Overview

This CRD provides a declarative way to:
- Assign realm roles to users
- Assign client roles to users
- Assign realm roles to groups
- Assign client roles to groups

## Examples

### Realm Role to User

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: admin-role-mapping
spec:
  subject:
    userRef:
      name: admin-user
  roleRef:
    name: admin-role
```

### Client Role to User

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: client-admin-mapping
spec:
  subject:
    userRef:
      name: service-user
  roleRef:
    name: manage-clients
  clientRef:
    name: my-client
```

### Inline Role Reference

Instead of referencing a `KeycloakRole` resource, you can specify the role name directly:

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: builtin-role-mapping
spec:
  subject:
    userRef:
      name: my-user
  role:
    name: offline_access
```

### Role to Group

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: group-role-mapping
spec:
  subject:
    groupRef:
      name: developers
  roleRef:
    name: developer-role
```

## Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `subject.userRef` | ResourceRef | Reference to KeycloakUser | Either userRef or groupRef |
| `subject.groupRef` | ResourceRef | Reference to KeycloakGroup | Either userRef or groupRef |
| `roleRef` | ResourceRef | Reference to KeycloakRole resource | Either roleRef or role |
| `role.name` | string | Keycloak role name (inline) | Either roleRef or role |
| `clientRef` | ResourceRef | Reference to KeycloakClient for client roles | No (realm role if omitted) |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the mapping is synced |
| `status` | string | Current status (Synced, Error, SubjectError, RoleError) |
| `message` | string | Additional status information |
| `subjectId` | string | Keycloak ID of the user/group |
| `roleId` | string | Keycloak ID of the role |
| `mappingType` | string | Type: UserRealmRole, UserClientRole, GroupRealmRole, GroupClientRole |

## Behavior

### Role Resolution

**Using `roleRef`:**
1. The operator looks up the referenced `KeycloakRole` resource
2. It retrieves the Keycloak role ID from the role's status
3. This is the recommended approach for roles managed by the operator

**Using `role.name`:**
1. The operator queries Keycloak for a role with the given name
2. This is useful for built-in roles like `offline_access`

### Mapping Types

| Subject | ClientRef | Result |
|---------|-----------|--------|
| userRef | - | User realm role mapping |
| userRef | set | User client role mapping |
| groupRef | - | Group realm role mapping |
| groupRef | set | Group client role mapping |

### Cleanup

When the `KeycloakRoleMapping` is deleted:
1. The finalizer removes the role mapping from Keycloak
2. The user/group no longer has the role assigned

## Use Cases

### RBAC Setup

Set up role-based access control with groups:

```yaml
# Create a group
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakGroup
metadata:
  name: admins
spec:
  realmRef: my-realm
  definition:
    name: admins
---
# Create a role
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRole
metadata:
  name: admin-role
spec:
  realmRef:
    name: my-realm
  definition:
    name: admin
    description: Full admin access
---
# Map role to group
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: admins-admin-role
spec:
  subject:
    groupRef:
      name: admins
  roleRef:
    name: admin-role
```

### Service Account Roles

Assign specific client roles to service accounts:

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: service-manage-users
spec:
  subject:
    userRef:
      name: service-account
  role:
    name: manage-users
  clientRef:
    name: realm-management
```

### Multiple Role Assignments

Assign multiple roles to the same user:

```yaml
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: user-role-1
spec:
  subject:
    userRef:
      name: my-user
  roleRef:
    name: role-1
---
apiVersion: keycloak.hostzero.io/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: user-role-2
spec:
  subject:
    userRef:
      name: my-user
  roleRef:
    name: role-2
```

## Notes

- Only one of `userRef` or `groupRef` can be specified
- Only one of `roleRef` or `role` can be specified
- When using `clientRef`, the role must be a client role, not a realm role
- Built-in Keycloak roles (like `offline_access`, `uma_authorization`) should use inline `role.name`
