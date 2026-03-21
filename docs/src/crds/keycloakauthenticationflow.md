# KeycloakAuthenticationFlow

A `KeycloakAuthenticationFlow` manages a Keycloak authentication flow and its execution tree via the Admin REST API.

> **Design note:** Unlike other CRDs in this operator that use a free-form `definition` field mirroring Keycloak's JSON representation, `KeycloakAuthenticationFlow` uses a **typed, structured spec**. This is a deliberate choice because Keycloak's authentication flow API is procedural -- there is no single endpoint to PUT a complete flow definition. The controller translates the spec into a sequence of API calls (create flow, add executions, set requirements, reorder, configure). The typed spec provides schema validation via `kubectl explain` and makes the nested flow/execution hierarchy natural to express in YAML.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: my-custom-browser
spec:
  # One of realmRef or clusterRealmRef must be specified
  realmRef:
    name: my-realm

  # Required: flow alias (unique within the realm)
  alias: my-custom-browser

  # Optional: human-readable description
  description: "Custom browser flow with MFA"

  # Required: flow type ("basic-flow" or "client-flow")
  providerId: basic-flow

  # Ordered list of executions
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
    - authenticator: auth-spnego
      requirement: DISABLED
    - subFlow:
        alias: my-browser-forms
        providerId: basic-flow
      requirement: ALTERNATIVE
      executions:
        - authenticator: auth-username-password-form
          requirement: REQUIRED
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  message: "Authentication flow synchronized"
  flowID: "12345678-1234-1234-1234-123456789abc"
  resourcePath: "/admin/realms/my-realm/authentication/flows/12345678-..."
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
```

## Examples

### Simple Direct Grant Flow

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-direct-grant
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  alias: custom-direct-grant
  description: "Custom direct grant with OTP"
  providerId: basic-flow
  executions:
    - authenticator: direct-grant-validate-username
      requirement: REQUIRED
    - authenticator: direct-grant-validate-password
      requirement: REQUIRED
    - authenticator: direct-grant-validate-otp
      requirement: REQUIRED
```

### Browser Flow with Conditional OTP

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-browser
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  alias: custom-browser
  description: "Browser flow with conditional OTP"
  providerId: basic-flow
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
    - authenticator: auth-spnego
      requirement: DISABLED
    - subFlow:
        alias: custom-browser-forms
        description: "Username/password with conditional OTP"
        providerId: basic-flow
      requirement: ALTERNATIVE
      executions:
        - authenticator: auth-username-password-form
          requirement: REQUIRED
        - subFlow:
            alias: custom-browser-conditional-otp
            providerId: basic-flow
          requirement: CONDITIONAL
          executions:
            - authenticator: conditional-user-configured
              requirement: REQUIRED
            - authenticator: auth-otp-form
              requirement: REQUIRED
              authenticatorConfig:
                otpHashAlgorithm: HmacSHA1
                otpLength: "6"
```

### With ClusterKeycloakRealm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-browser
  namespace: keycloak
spec:
  clusterRealmRef:
    name: my-cluster-realm
  alias: custom-browser
  providerId: basic-flow
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
    - authenticator: auth-username-password-form
      requirement: REQUIRED
```

## Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | object | One of realmRef/clusterRealmRef | Reference to a KeycloakRealm |
| `clusterRealmRef` | object | One of realmRef/clusterRealmRef | Reference to a ClusterKeycloakRealm |
| `alias` | string | Yes | Unique flow alias within the realm |
| `description` | string | No | Human-readable description |
| `providerId` | string | Yes | Flow type: `basic-flow` or `client-flow` |
| `executions` | array | No | Ordered list of executions |

### Execution Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `authenticator` | string | Mutually exclusive with `subFlow` | Authenticator provider ID |
| `subFlow` | object | Mutually exclusive with `authenticator` | Nested sub-flow definition |
| `requirement` | string | Yes | `REQUIRED`, `ALTERNATIVE`, `DISABLED`, or `CONDITIONAL` |
| `executions` | array | No | Child executions (only when `subFlow` is set) |
| `authenticatorConfig` | map | No | Key-value config for the authenticator |

### SubFlow Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `alias` | string | Yes | Unique sub-flow alias |
| `description` | string | No | Human-readable description |
| `providerId` | string | Yes | Sub-flow type: `basic-flow` or `client-flow` |

## Common Authenticator Provider IDs

| Provider ID | Description |
|------------|-------------|
| `auth-cookie` | Cookie-based authentication |
| `auth-spnego` | Kerberos/SPNEGO |
| `auth-username-password-form` | Username/password form |
| `auth-otp-form` | OTP form |
| `conditional-user-configured` | Condition: user has configured the authenticator |
| `direct-grant-validate-username` | Validate username (direct grant) |
| `direct-grant-validate-password` | Validate password (direct grant) |
| `direct-grant-validate-otp` | Validate OTP (direct grant) |
| `identity-provider-redirector` | Redirect to identity provider |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcaf` | `keycloakauthenticationflows` |

```bash
kubectl get kcaf
```

## Notes

- **Phase 1 behavior:** On spec changes, the flow is deleted and recreated. Incremental updates (adding/removing individual executions) will be added in a future release.
- Deleting the CR deletes the flow from Keycloak (unless the `keycloak.hostzero.com/preserve-resource` annotation is set).
- Authentication flows created by this CRD are not built-in and can be freely managed.
- To use a custom flow as the realm's browser/direct-grant/etc. flow, you currently need to configure that binding in the `KeycloakRealm` definition. Flow binding support will be added in a future release.
