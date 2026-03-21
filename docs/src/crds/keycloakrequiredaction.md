# KeycloakRequiredAction

A `KeycloakRequiredAction` manages a required action provider within a Keycloak realm. Required actions are steps that users must complete (e.g. update password, configure OTP, verify email) and can be enabled, disabled, or set as default for new users.

Changes to `requiredActions` in `KeycloakRealm.spec.definition` only take effect on initial realm import. This CRD uses the dedicated required action API endpoints to allow changes after realm creation.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: my-terms-and-conditions
spec:
  # One of realmRef or clusterRealmRef must be specified

  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm

  # Option 2: Reference to a ClusterKeycloakRealm
  # clusterRealmRef:
  #   name: my-cluster-realm

  # Required: RequiredActionProviderRepresentation
  definition:
    alias: TERMS_AND_CONDITIONS
    name: "Terms and Conditions"
    providerId: TERMS_AND_CONDITIONS
    enabled: true
    defaultAction: true
    priority: 20
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  alias: "TERMS_AND_CONDITIONS"
  message: "Required action synchronized"
  resourcePath: "/admin/realms/my-realm/authentication/required-actions/TERMS_AND_CONDITIONS"
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
```

## Examples

### Enable and Default Terms & Conditions

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: terms-and-conditions
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    alias: TERMS_AND_CONDITIONS
    name: "Terms and Conditions"
    providerId: TERMS_AND_CONDITIONS
    enabled: true
    defaultAction: true
    priority: 20
```

### Configure OTP as Required

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: configure-otp
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    alias: CONFIGURE_TOTP
    name: "Configure OTP"
    providerId: CONFIGURE_TOTP
    enabled: true
    defaultAction: true
    priority: 10
```

### Verify Email

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: verify-email
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    alias: VERIFY_EMAIL
    name: "Verify Email"
    providerId: VERIFY_EMAIL
    enabled: true
    defaultAction: false
    priority: 50
```

### Update Password

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: update-password
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    alias: UPDATE_PASSWORD
    name: "Update Password"
    providerId: UPDATE_PASSWORD
    enabled: true
    defaultAction: false
    priority: 30
```

### With ClusterKeycloakRealm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRequiredAction
metadata:
  name: verify-email
  namespace: keycloak
spec:
  clusterRealmRef:
    name: my-cluster-realm
  definition:
    alias: VERIFY_EMAIL
    name: "Verify Email"
    providerId: VERIFY_EMAIL
    enabled: true
    defaultAction: true
```

## Definition Properties

The `definition` field accepts any valid Keycloak [RequiredActionProviderRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html):

| Field | Type | Description |
|-------|------|-------------|
| `alias` | string | Unique alias for the required action (e.g. `VERIFY_EMAIL`) |
| `name` | string | Display name |
| `providerId` | string | Provider ID (usually same as alias) |
| `enabled` | boolean | Whether the required action is enabled |
| `defaultAction` | boolean | Whether new users get this action by default |
| `priority` | integer | Ordering priority (lower = higher priority) |
| `config` | map | Provider-specific configuration |

## Common Required Action Aliases

| Alias | Description |
|-------|-------------|
| `UPDATE_PASSWORD` | Force password update |
| `CONFIGURE_TOTP` | Configure OTP authenticator |
| `VERIFY_EMAIL` | Verify email address |
| `UPDATE_PROFILE` | Update user profile |
| `VERIFY_PROFILE` | Verify user profile |
| `TERMS_AND_CONDITIONS` | Accept terms and conditions |
| `delete_account` | Allow account self-deletion |
| `webauthn-register` | Register WebAuthn security key |
| `webauthn-register-passwordless` | Register WebAuthn passwordless credential |
| `update_user_locale` | Update user locale |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcra` | `keycloakrequiredactions` |

```bash
kubectl get kcra
```

## Notes

- Most built-in required actions are pre-registered in Keycloak. This CRD will update them if they already exist, or register and configure them if they don't.
- Deleting the CR deletes the required action from Keycloak (unless the `keycloak.hostzero.com/preserve-resource` annotation is set).
- The `priority` field controls the order in which required actions are presented to the user.
