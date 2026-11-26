# KeycloakUser

> **Identifier field:** Set the username in the `spec.username` field. Required for regular users and immutable once set; omit it for service account users, which are identified by `clientRef`. A `username` inside `spec.definition` is tolerated only when it matches `spec.username`; a conflicting value is rejected.

A `KeycloakUser` represents a user within a Keycloak realm, or a service account user associated with a client.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: john-doe
spec:
  # One of realmRef, clusterRealmRef, or clientRef must be specified
  
  # Option 1: Reference to a KeycloakRealm (for regular realm users)
  realmRef:
    name: my-realm
  
  # Option 2: Reference to a ClusterKeycloakRealm (for cluster-scoped realms)
  # clusterRealmRef:
  #   name: my-cluster-realm
  
  # Option 3: Reference to a KeycloakClient (for service account users)
  # clientRef:
  #   name: my-client
  
  # User definition (Keycloak UserRepresentation)
  # Note: For service account users (clientRef), definition is optional
  username: johndoe
  definition:
    email: john.doe@example.com
    firstName: John
    lastName: Doe
    enabled: true
    # ... any other Keycloak user properties
    # (must not contain realmRoles, clientRoles, groups, or requiredActions — see below)
  
  # Optional: role and group assignments (see "Roles and Groups")
  realmRoles:
    - offline_access
  clientRoles:
    my-app:
      - admin
  groups:
    - developers
  
  # Optional: Initial password (only set on creation)
  # Exactly one of value or valueFromSecret must be set
  initialPassword:
    value: "temporary-password"
    # valueFromSecret:
    #   secretName: john-doe-password
    #   secretKey: password  # default: password
    temporary: true  # User must change on first login

  # Optional: required actions queued when the operator creates the user
  # (see "Required Actions")
  initialRequiredActions:
    - VERIFY_EMAIL
```

To manage credentials in a Kubernetes Secret (including password generation), use [KeycloakUserCredential](./keycloakusercredential.md).

## Status

```yaml
status:
  ready: true
  status: "Ready"
  userID: "12345678-1234-1234-1234-123456789abc"
  message: "User synchronized successfully"
  resourcePath: "/admin/realms/my-realm/users/12345678-..."
  isServiceAccount: false
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

### Basic User

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: admin-user
spec:
  realmRef:
    name: my-realm
  username: admin
  definition:
    email: admin@example.com
    firstName: Admin
    lastName: User
    enabled: true
    emailVerified: true
```

### User with Managed Credentials

Create the user, then attach a `KeycloakUserCredential` that generates a password, stores it in a Secret, and syncs it into Keycloak:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: john-doe
spec:
  realmRef:
    name: my-realm
  username: johndoe
  definition:
    email: john.doe@example.com
    firstName: John
    lastName: Doe
    enabled: true
    emailVerified: true
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: john-doe-credential
spec:
  userRef:
    name: john-doe
  userSecret:
    secretName: john-doe-password
    create: true  # Generate a password and create the secret
```

See [KeycloakUserCredential](./keycloakusercredential.md) for existing secrets, key names, and password policy.

### User with Attributes

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: employee
spec:
  realmRef:
    name: my-realm
  username: jsmith
  definition:
    email: jsmith@company.com
    firstName: Jane
    lastName: Smith
    enabled: true
    attributes:
      department:
        - Engineering
      employee_id:
        - "12345"
      manager:
        - "jdoe"
```

### User with Roles and Groups

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: developer
spec:
  realmRef:
    name: my-realm
  username: developer1
  definition:
    email: dev@example.com
    enabled: true
  realmRoles:
    - offline_access
  clientRoles:
    my-app:
      - viewer
  groups:
    - developers
    - /engineering/team-alpha   # nested group, referenced by full path
```

### Service Account User

Manage the service account user associated with a client. This is useful for assigning roles or attributes to a client's service account.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: my-service-account
spec:
  # Use clientRef instead of realmRef for service account users
  clientRef:
    name: my-service-client
  # Definition is optional - the service account is automatically created by Keycloak
  # when serviceAccountsEnabled: true on the client
  definition:
    # You can add/modify attributes on the service account
    attributes:
      department:
        - Platform
```

### Service Account with Roles

The typed role fields also work for service account users:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: my-service-sa
spec:
  clientRef:
    name: my-service-client
  realmRoles:
    - admin
  clientRoles:
    realm-management:
      - manage-users
```

Alternatively, assign individual roles with `KeycloakRoleMapping` using `serviceAccountRef` — without an intermediate `KeycloakUser` (see [KeycloakRoleMapping](./keycloakrolemapping.md)). Do not combine both mechanisms for the same user.

## Roles and Groups

`spec.realmRoles`, `spec.clientRoles`, and `spec.groups` are reconciled via Keycloak's dedicated role-mapping and group-membership endpoints. They must not appear inside `spec.definition` (Keycloak ignores them in the user representation anyway); the operator rejects such definitions.

Each field is **authoritative when set**:

- Omitted (`nil`): the operator does not touch that category of assignments.
- Set (even to an empty list): the operator reconciles Keycloak to exactly that set, removing anything else. For `clientRoles`, roles on clients absent from the map are also removed.

Groups are referenced by full path (e.g. `/engineering/team-alpha` for a nested group). An entry without a leading `/` is treated as a top-level group, so `developers` is the same as `/developers`. Roles and groups must already exist in the realm (e.g. via `KeycloakRole` or `KeycloakGroup`); unknown names are skipped with a log message.

Because these fields are authoritative, do not combine them with `KeycloakRoleMapping` resources targeting the same user — the user's reconciler would remove the mappings again.

## Required Actions

`spec.initialRequiredActions` lists the required actions, such as `UPDATE_PASSWORD`, `VERIFY_EMAIL` or `CONFIGURE_TOTP`, that are queued for the user when the operator creates it:

```yaml
spec:
  realmRef:
    name: my-realm
  username: johndoe
  initialRequiredActions:
    - UPDATE_PASSWORD
  definition:
    email: john.doe@example.com
    enabled: true
```

A required action is not a setting. It is an entry in a queue, and Keycloak removes it as soon as the user has completed it. The list is therefore sent with the create request only:

- It is never applied again, so a completed action does not come back.
- Changing the list after the user exists has no effect.
- It is not applied to a user that already exists in Keycloak when the resource is created.
- Updates leave Keycloak's queue alone. Actions added in the Admin Console, or the `UPDATE_PASSWORD` that `initialPassword.temporary` adds, stay until the user completes them.
- It cannot be combined with `clientRef`: the operator does not create service account users.

`requiredActions` must not appear inside `spec.definition`. The operator rejects such a definition with `Ready=False` and the reason `InvalidDefinition`, and does not reconcile the user until the key is removed.

> **Upgrading:** up to v0.14 the key was accepted in the definition and written to Keycloak on every reconcile, which put a completed action back and made the user repeat it at the next login. A definition that still carries the key, an empty list included, is now rejected. Move the actions to `spec.initialRequiredActions`, or drop the key. Manifests written by the export command of an earlier release contain `requiredActions` for every user and need the same change.

## Definition Properties

Common properties from [Keycloak UserRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#UserRepresentation):

| Property | Type | Description |
|----------|------|-------------|
| `username` | string | Username (required) |
| `email` | string | Email address |
| `firstName` | string | First name |
| `lastName` | string | Last name |
| `enabled` | boolean | Whether user is enabled |
| `emailVerified` | boolean | Email verified flag |
| `attributes` | map | Custom user attributes |

Role and group assignments (`realmRoles`, `clientRoles`, `groups`) are typed spec fields, not definition properties. Required actions go in `spec.initialRequiredActions` (see [Required Actions](#required-actions)).

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcu` | `keycloakusers` |

```bash
kubectl get kcu
```

## Parent Reference

A `KeycloakUser` can belong to one of three parent types:

| Reference | Use Case | Parent Type |
|-----------|----------|-------------|
| `realmRef` | Regular realm users | KeycloakRealm |
| `clusterRealmRef` | Users in cluster-scoped realms | ClusterKeycloakRealm |
| `clientRef` | Service account users | KeycloakClient |

**Note:** Exactly one of `realmRef`, `clusterRealmRef`, or `clientRef` must be specified.

### Service Account Users

When using `clientRef`, the operator manages the service account user that Keycloak automatically creates for clients with `serviceAccountsEnabled: true`. This allows you to:

- Add custom attributes to the service account
- Use `KeycloakRoleMapping` to assign roles to the service account
- Manage the service account declaratively alongside other resources

The `definition` field is optional for service account users since Keycloak creates the user automatically.

## Notes

- `spec.initialPassword` is only set on user creation
- `spec.initialPassword.valueFromSecret` reads the password from a Secret in the same namespace as the user. If the Secret does not exist yet, the user is not created in Keycloak until it appears
- `spec.initialRequiredActions` is only sent when the operator creates the user
- To manage or rotate a password declaratively, use [KeycloakUserCredential](./keycloakusercredential.md)
- For service account users, the username is automatically set by Keycloak (format: `service-account-<client-id>`)
