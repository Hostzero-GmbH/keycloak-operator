# KeycloakUserCredential

The `KeycloakUserCredential` resource manages user credentials (passwords) in Keycloak via Kubernetes Secrets.

## Overview

This CRD provides a way to:
- Store user passwords in Kubernetes Secrets
- Automatically create secrets with generated passwords
- Sync passwords to Keycloak users
- Manage password policies

## Example

### Using an existing Secret

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: user-credential
spec:
  userRef:
    name: my-user
  userSecret:
    secretName: my-user-credentials
    usernameKey: username
    passwordKey: password
```

### Auto-creating a Secret

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: user-credential
spec:
  userRef:
    name: my-user
  userSecret:
    secretName: my-user-credentials
    create: true
    usernameKey: username
    passwordKey: password
    passwordPolicy:
      length: 24
      includeSymbols: true
      includeNumbers: true
```

## Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `userRef` | ResourceRef | Reference to the KeycloakUser resource | Yes |
| `userSecret.secretName` | string | Name of the Kubernetes Secret | Yes |
| `userSecret.create` | boolean | Create secret if it doesn't exist | No (default: false) |
| `userSecret.usernameKey` | string | Key in secret for username | No (default: "username") |
| `userSecret.passwordKey` | string | Key in secret for password | No (default: "password") |
| `userSecret.emailKey` | string | Key in secret for email | No |
| `userSecret.passwordPolicy.length` | int | Length of generated password | No (default: 24) |
| `userSecret.passwordPolicy.includeNumbers` | boolean | Include numbers in password | No (default: true) |
| `userSecret.passwordPolicy.includeSymbols` | boolean | Include symbols in password | No (default: true) |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the credential is synced |
| `status` | string | Current status (Synced, Error, SecretError) |
| `message` | string | Additional status information |
| `resourcePath` | string | Keycloak API path for the user |
| `secretCreated` | boolean | Whether the secret was created by the operator |
| `passwordHash` | string | Hash of the last synchronized password |
| `secretResourceVersion` | string | Resource version of the secret when last synced |
| `instance` | object | Resolved instance reference |
| `realm` | object | Resolved realm reference |
| `observedGeneration` | integer | Last observed generation |
| `conditions` | []Condition | Kubernetes conditions |

## Behavior

### Secret Creation

When `create: true` is set:
1. The operator creates a new Secret if it doesn't exist
2. A password is generated according to the password policy
3. The username is set to match the Keycloak user's username

### Password Sync

When the Secret exists (created or pre-existing):
1. The operator reads the password from the Secret
2. The password is set in Keycloak for the referenced user
3. The `passwordHash` is updated for change detection

### Cleanup

When the `KeycloakUserCredential` is deleted:
- If `secretCreated: true` in status, the Secret is also deleted (via owner references)
- Pre-existing secrets are not deleted

## Use Cases

### Initial User Setup

Create users with auto-generated passwords:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: new-user
spec:
  realmRef:
    name: my-realm
  definition:
    username: new-user
    email: user@example.com
    enabled: true
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: new-user-creds
spec:
  userRef:
    name: new-user
  userSecret:
    secretName: new-user-password
    create: true
```

### Service Account Passwords

Manage service account credentials that can be mounted into pods:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUserCredential
metadata:
  name: service-account-creds
spec:
  userRef:
    name: service-account-user
  userSecret:
    secretName: app-keycloak-credentials
    create: true
    passwordPolicy:
      length: 32
      includeSymbols: false
```
