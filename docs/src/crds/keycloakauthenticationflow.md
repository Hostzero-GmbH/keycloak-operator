# KeycloakAuthenticationFlow

A `KeycloakAuthenticationFlow` manages a Keycloak authentication flow and its execution tree via the Admin REST API.

The top-level fields (`alias`, `description`, `providerId`, realm references) are typed; the `executions` tree is a free-form JSON value with arbitrary nesting depth.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: my-custom-browser
spec:
  # One of realmRef or clusterRealmRef
  realmRef:
    name: my-realm

  # Required: flow alias (unique within the realm)
  alias: my-custom-browser

  # Optional human-readable description
  description: "Custom browser flow with MFA"

  # Top-level flow type. "basic-flow" or "client-flow" for top-level flows;
  # the controller does not constrain the value, so future Keycloak provider
  # types are usable without an operator release.
  providerId: basic-flow

  # Ordered list of executions. Each entry is either a leaf authenticator
  # or a sub-flow; sub-flows recurse to arbitrary depth.
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
    - authenticator: auth-spnego
      requirement: DISABLED
    - subFlow:
        alias: my-browser-forms
        providerId: basic-flow
        executions:
          - authenticator: auth-username-password-form
            requirement: REQUIRED
      requirement: ALTERNATIVE
```

## Execution shape

Each entry in an `executions` list is one of two shapes.

### Leaf authenticator

```yaml
- authenticator: auth-cookie       # Keycloak provider ID
  requirement: ALTERNATIVE         # REQUIRED | ALTERNATIVE | DISABLED | CONDITIONAL
  authenticatorConfig:             # optional, applied after creation
    someKey: someValue
```

### Sub-flow

```yaml
- subFlow:
    alias: forms                   # required, unique within the parent
    providerId: basic-flow         # "basic-flow", "client-flow", or "form-flow"
    description: "Optional"
    executions:                    # child executions live here (inline shape)
      - authenticator: auth-username-password-form
        requirement: REQUIRED
  requirement: ALTERNATIVE
```

The same sub-flow can also be expressed with executions placed next to `subFlow` instead of inside it (this matches Keycloak's own realm export format):

```yaml
- subFlow:
    alias: forms
    providerId: basic-flow
  requirement: ALTERNATIVE
  executions:                       # child executions live here (sibling shape)
    - authenticator: auth-username-password-form
      requirement: REQUIRED
```

If both lists are present, the inline list precedes the sibling list. Within each list, declaration order is preserved.

### Sub-flow `providerId` values

| Value | When to use |
|---|---|
| `basic-flow` | A regular sequence of authenticator/sub-flow steps. The most common choice. |
| `client-flow` | Used for client authentication flows (top-level). |
| `form-flow` | A sub-flow that aggregates `FormAction` providers into a single rendered form. **Required** when the children are form actions such as `registration-user-creation`, `registration-profile-action`, `registration-password-action`, `registration-recaptcha`. These will not work inside a `basic-flow` sub-flow. |

The CRD does not enumerate the allowed values so future Keycloak releases that introduce new provider types do not require an operator update.

## Examples

### Direct grant flow

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-direct-grant
spec:
  realmRef:
    name: my-realm
  alias: custom-direct-grant
  providerId: basic-flow
  executions:
    - authenticator: direct-grant-validate-username
      requirement: REQUIRED
    - authenticator: direct-grant-validate-password
      requirement: REQUIRED
    - authenticator: direct-grant-validate-otp
      requirement: REQUIRED
```

### Browser flow with conditional OTP (deeply nested)

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-browser
spec:
  realmRef:
    name: my-realm
  alias: custom-browser
  providerId: basic-flow
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
    - subFlow:
        alias: custom-browser-forms
        providerId: basic-flow
        executions:
          - authenticator: auth-username-password-form
            requirement: REQUIRED
          - subFlow:
              alias: custom-browser-conditional-otp
              providerId: basic-flow
              executions:
                - authenticator: conditional-user-configured
                  requirement: REQUIRED
                - authenticator: auth-otp-form
                  requirement: REQUIRED
                  authenticatorConfig:
                    otpHashAlgorithm: HmacSHA1
                    otpLength: "6"
            requirement: CONDITIONAL
      requirement: ALTERNATIVE
```

### Registration flow with `form-flow` sub-flow

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: custom-registration
spec:
  realmRef:
    name: my-realm
  alias: custom-registration
  providerId: basic-flow
  executions:
    - subFlow:
        alias: custom-registration-form
        providerId: form-flow
      requirement: REQUIRED
      executions:
        - authenticator: registration-user-creation
          requirement: REQUIRED
        - authenticator: registration-password-action
          requirement: REQUIRED
        - authenticator: registration-terms-and-conditions
          requirement: DISABLED
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

If the `executions` payload is malformed (missing `requirement`, both `authenticator` and `subFlow` set, missing sub-flow `alias`/`providerId`, etc.) the controller sets `status.status = "InvalidSpec"` and `status.message` with a JSON-pointer-style path to the offending node, e.g. `[1].executions[0].requirement is required`.

## Spec fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | object | one of `realmRef` / `clusterRealmRef` | Reference to a `KeycloakRealm` |
| `clusterRealmRef` | object | one of `realmRef` / `clusterRealmRef` | Reference to a `ClusterKeycloakRealm` |
| `alias` | string | yes | Unique flow alias within the realm |
| `description` | string | no | Human-readable description |
| `providerId` | string | yes | Top-level flow type (`basic-flow`, `client-flow`, …) |
| `executions` | JSON array | no | Ordered list of executions; see [Execution shape](#execution-shape) |

## Common authenticator provider IDs

| Provider ID | Description |
|------------|-------------|
| `auth-cookie` | Cookie-based authentication |
| `auth-spnego` | Kerberos / SPNEGO |
| `auth-username-password-form` | Username/password form |
| `auth-otp-form` | OTP form |
| `conditional-user-configured` | Condition: user has configured the authenticator |
| `direct-grant-validate-username` | Validate username (direct grant) |
| `direct-grant-validate-password` | Validate password (direct grant) |
| `direct-grant-validate-otp` | Validate OTP (direct grant) |
| `identity-provider-redirector` | Redirect to identity provider |
| `registration-user-creation` | Form action: create user (registration `form-flow` only) |
| `registration-password-action` | Form action: set password (registration `form-flow` only) |
| `registration-terms-and-conditions` | Form action: terms & conditions (registration `form-flow` only) |
| `registration-recaptcha` | Form action: reCAPTCHA (registration `form-flow` only) |

## Short names

```bash
kubectl get kcaf       # KeycloakAuthenticationFlow
```

## Behavior on update

Every reconcile (spec change or periodic resync) reads the live execution tree from Keycloak and converges it with the spec, so external drift — admin UI edits, `kcadm` reorders, realm re-imports — is reverted on the next pass. The top-level flow ID stays stable across updates so flows referenced as a sub-flow execution or as a realm binding (`browserFlow`, `registrationFlow`, etc.) keep working.

The reconciler walks the desired tree against the live tree and applies the minimum set of API calls:

- **Identity rules:**
  - leaf executions are matched by their `authenticator` provider id (e.g. `auth-cookie`)
  - sub-flow executions are matched by `subFlow.alias`
  - leaf and sub-flow with the same name are *not* matched against each other
- **Adds:** a desired entry with no live counterpart is created with `addExecution` / `addSubFlow`.
- **Removes:** a live entry with no desired counterpart is deleted via `DELETE /authentication/executions/{id}`. Inline sub-flow executions are deleted the same way; the top-level flow itself is never deleted on a spec change.
- **Updates on matched entries:**
  - if `requirement` differs, it is patched via `PUT /authentication/flows/{alias}/executions`.
  - leaf `authenticatorConfig` is converged: created when the spec adds it, deleted when the spec drops it, and updated in place via `PUT /authentication/config/{id}` when the key/value contents change.
  - sub-flow nodes recurse — children of matched sub-flows are reconciled the same way.
- **Reorder:** at the end of each level, the controller assigns explicit `priority` values to every child via `PUT /authentication/flows/{alias}/executions` so the live order matches the spec deterministically. See [Known limitations](#known-limitations) for the Keycloak version requirement.

A one-line summary of what changed (`added=N updated=M removed=K reorderedParents=R`) is logged for each update.

Duplicate identities at the same level are handled occurrence-by-occurrence: the i-th desired entry with identity X matches the i-th unmatched live entry with the same identity. Extras on either side surface as adds or removes.

### Hard limits

Two changes cannot be applied in place and are reported as failures with a clear status:

- **Top-level `providerId` change** (e.g. `basic-flow` → `client-flow`): Keycloak does not support swapping the provider type of an existing flow. The status is set to `ProviderChangeUnsupported` and the message tells you to pick a new alias.
- **Renaming the top-level `alias`**: the controller treats this as "old flow + new flow"; the old flow has to be removed by deleting its `KeycloakAuthenticationFlow` resource.

## Known limitations

### Execution ordering requires Keycloak 25+

Order enforcement relies on the `priority` field added to `PUT /authentication/flows/{alias}/executions` in [keycloak/keycloak#27751](https://github.com/keycloak/keycloak/pull/27751). On Keycloak 24 and older the field is silently dropped, so the operator cannot enforce or repair execution order on those versions. Initial order still matches the spec there because pre-25 Keycloak assigned sequential priorities on add. Other drift (adds, removes, requirement changes, config changes) is detected and repaired on every Keycloak version.

## Notes

- Deleting the CR deletes the flow from Keycloak unless the `keycloak.hostzero.com/preserve-resource` annotation is set.
- Authentication flows created by this CRD are not built-in and can be freely managed.
- To use a custom flow as the realm's `browserFlow` / `registrationFlow` / `directGrantFlow` / `resetCredentialsFlow` / `clientAuthenticationFlow` / `dockerAuthenticationFlow`, set those bindings in the `KeycloakRealm` definition. Keycloak rejects realm imports referencing a flow alias that does not exist yet (see [keycloak/keycloak#23980](https://github.com/keycloak/keycloak/issues/23980)). The operator works around that by stripping these bindings on the *first* `CreateRealm` call, marking the realm `Ready`, and re-applying them on subsequent reconciles. The realm controller also watches `KeycloakAuthenticationFlow` resources and requeues the realm immediately when a referenced flow is created, so bindings converge without long retry windows.
