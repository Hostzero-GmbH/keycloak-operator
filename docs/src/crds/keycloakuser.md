# KeycloakUser

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
    namespace: default  # Optional
  
  # Option 2: Reference to a ClusterKeycloakRealm (for cluster-scoped realms)
  # clusterRealmRef:
  #   name: my-cluster-realm
  
  # Option 3: Reference to a KeycloakClient (for service account users)
  # clientRef:
  #   name: my-client
  #   namespace: default  # Optional
  
  # User definition (Keycloak UserRepresentation)
  # Note: For service account users (clientRef), definition is optional
  definition:
    username: johndoe
    email: john.doe@example.com
    firstName: John
    lastName: Doe
    enabled: true
    # ... any other Keycloak user properties
  
  # Optional: Initial password (only set on creation)
  initialPassword:
    value: "temporary-password"
    temporary: true  # User must change on first login
  
  # Optional: Password configuration via Kubernetes Secret
  userSecret:
    secretName: john-doe-password
    usernameKey: username   # Default: username
    passwordKey: password   # Default: password
    generatePassword: true  # Auto-generate a password
```

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
  definition:
    username: admin
    email: admin@example.com
    firstName: Admin
    lastName: User
    enabled: true
    emailVerified: true
```

### User with Credentials

First, create a secret with the password:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: john-password
type: Opaque
stringData:
  password: "secure-password-123"
```

Then create the user:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: john-doe
spec:
  realmRef:
    name: my-realm
  definition:
    username: johndoe
    email: john.doe@example.com
    firstName: John
    lastName: Doe
    enabled: true
    emailVerified: true
  userSecret:
    secretName: john-password
```

### User with Attributes

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: employee
spec:
  realmRef:
    name: my-realm
  definition:
    username: jsmith
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

### User with Groups

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: developer
spec:
  realmRef:
    name: my-realm
  definition:
    username: developer1
    email: dev@example.com
    enabled: true
    groups:
      - /developers
      - /team-alpha
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

### Service Account with Role Mapping

Combine `KeycloakUser` (via `clientRef`) with `KeycloakRoleMapping` to assign roles to a service account:

```yaml
# First, define the service account user
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: my-service-sa
spec:
  clientRef:
    name: my-service-client
---
# Assign a realm role to the service account
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: my-service-sa-admin
spec:
  subject:
    userRef:
      name: my-service-sa
  role:
    name: admin
---
# Assign a client role to the service account
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRoleMapping
metadata:
  name: my-service-sa-manage-users
spec:
  subject:
    userRef:
      name: my-service-sa
  role:
    name: manage-users
    clientRef:
      name: realm-management
```

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
| `groups` | string[] | Group paths to join |
| `requiredActions` | string[] | Required actions on login |

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

- Passwords are only set on user creation
- To update a password, delete and recreate the user, or use Keycloak's admin console
- Group memberships specified in `groups` are resolved by path
- For service account users, the username is automatically set by Keycloak (format: `service-account-<client-id>`)
