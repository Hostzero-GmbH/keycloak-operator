# KeycloakRealm

A `KeycloakRealm` represents a realm within a Keycloak instance.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  # One of instanceRef or clusterInstanceRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakInstance
  instanceRef:
    name: my-keycloak
  
  # Option 2: Reference to a ClusterKeycloakInstance
  # clusterInstanceRef:
  #   name: my-cluster-instance
  
  # Optional: Realm name in Keycloak (defaults to metadata.name)
  realmName: my-realm
  
  # Required: Realm definition (Keycloak RealmRepresentation)
  definition:
    realm: my-realm
    displayName: My Realm
    enabled: true
    # ... any other Keycloak realm properties
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  message: "Realm synchronized successfully"
  resourcePath: "/admin/realms/my-realm"
  instance:
    instanceRef: my-keycloak
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
```

## Example

### Basic Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app-realm
spec:
  instanceRef:
    name: production-keycloak
  definition:
    realm: my-app
    displayName: My Application
    enabled: true
```

### With ClusterKeycloakInstance

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app-realm
spec:
  clusterInstanceRef:
    name: central-keycloak
  definition:
    realm: my-app
    displayName: My Application
    enabled: true
```

### Full Configuration

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: production-realm
spec:
  instanceRef:
    name: production-keycloak
  definition:
    realm: production
    displayName: Production Realm
    enabled: true
    
    # Login settings
    registrationAllowed: false
    registrationEmailAsUsername: true
    loginWithEmailAllowed: true
    duplicateEmailsAllowed: false
    resetPasswordAllowed: true
    rememberMe: true
    
    # Session settings
    ssoSessionIdleTimeout: 1800
    ssoSessionMaxLifespan: 36000
    accessTokenLifespan: 300
    
    # Security settings
    bruteForceProtected: true
    permanentLockout: false
    maxFailureWaitSeconds: 900
    minimumQuickLoginWaitSeconds: 60
    waitIncrementSeconds: 60
    quickLoginCheckMilliSeconds: 1000
    maxDeltaTimeSeconds: 43200
    failureFactor: 5
    
    # Themes
    loginTheme: keycloak
    accountTheme: keycloak
    adminTheme: keycloak
    emailTheme: keycloak
    
    # SMTP settings (non-sensitive parts in definition)
    smtpServer:
      host: smtp.example.com
      port: "587"
      fromDisplayName: My App
      from: noreply@example.com
      starttls: "true"
      auth: "true"
  
  # SMTP credentials from a Kubernetes Secret (recommended over plaintext in definition)
  smtpSecretRef:
    name: my-smtp-credentials
    userKey: user         # optional, defaults to "user"
    passwordKey: password # optional, defaults to "password"
```

### SMTP Credentials from Secret

To avoid storing SMTP credentials in plaintext in the CR, use `smtpSecretRef` to reference a Kubernetes Secret:

```bash
kubectl create secret generic smtp-credentials \
  --from-literal=user=smtp-user@example.com \
  --from-literal=password=my-smtp-password
```

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef:
    name: my-keycloak
  smtpSecretRef:
    name: smtp-credentials
    # userKey: user         # default
    # passwordKey: password # default
  definition:
    realm: my-realm
    enabled: true
    smtpServer:
      host: smtp.example.com
      port: "587"
      from: noreply@example.com
      starttls: "true"
      auth: "true"
```

The operator reads the `user` and `password` values from the referenced secret and injects them into `smtpServer` before sending the realm configuration to Keycloak. The secret must exist in the same namespace as the `KeycloakRealm`.

For `ClusterKeycloakRealm`, the secret namespace must be specified explicitly:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: my-realm
spec:
  clusterInstanceRef:
    name: central-keycloak
  smtpSecretRef:
    name: smtp-credentials
    namespace: keycloak-system
  definition:
    realm: my-realm
    enabled: true
    smtpServer:
      host: smtp.example.com
      port: "587"
      from: noreply@example.com
      starttls: "true"
      auth: "true"
```

When the referenced secret changes, the operator automatically re-reconciles the realm to pick up the new credentials.

## Definition Properties

The `definition` field accepts any property from the [Keycloak RealmRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#RealmRepresentation).

Common properties:

| Property | Type | Description |
|----------|------|-------------|
| `realm` | string | Realm name (required) |
| `displayName` | string | Display name for the realm |
| `enabled` | boolean | Whether the realm is enabled |
| `registrationAllowed` | boolean | Allow user registration |
| `loginWithEmailAllowed` | boolean | Allow login with email |
| `ssoSessionIdleTimeout` | integer | SSO session idle timeout (seconds) |
| `accessTokenLifespan` | integer | Access token lifespan (seconds) |

## Binding Custom Authentication Flows

A realm definition may bind built-in authentication points to custom flows via `browserFlow`, `registrationFlow`, `directGrantFlow`, `resetCredentialsFlow`, `clientAuthenticationFlow`, or `dockerAuthenticationFlow`. Keycloak rejects realm imports that reference a flow alias which does not yet exist (see [keycloak/keycloak#23980](https://github.com/keycloak/keycloak/issues/23980)), which would otherwise prevent declaratively creating the realm and the flow at the same time.

The operator works around this with **deferred bindings**:

1. On the *first* `CreateRealm` call, any flow-binding fields whose target alias does not yet exist in Keycloak are stripped before the request is sent. The realm is created and marked `Ready`; the operator records that bindings were deferred.
2. The realm controller watches `KeycloakAuthenticationFlow` resources and requeues the realm immediately when a referenced flow becomes ready, instead of waiting for the next periodic resync.
3. On the next reconcile (either triggered by the watch or by the periodic resync) the operator updates the realm with the original bindings now that the referenced flows exist.

Practically this means you can apply a `KeycloakRealm` and its `KeycloakAuthenticationFlow` resources together — in any order — and convergence happens within seconds.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
    browserFlow: my-custom-browser           # may be created later
    registrationFlow: my-custom-registration # may be created later
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakAuthenticationFlow
metadata:
  name: my-custom-browser
spec:
  realmRef:
    name: my-realm
  alias: my-custom-browser
  providerId: basic-flow
  executions:
    - authenticator: auth-cookie
      requirement: ALTERNATIVE
```

See [KeycloakAuthenticationFlow](./keycloakauthenticationflow.md) for the flow CRD reference.

## Identity Providers and Mappers

Identity providers themselves can be embedded in `definition.identityProviders` for *initial* realm creation, but Keycloak's `PUT /admin/realms/{realm}` endpoint does **not** consume the `identityProviders` or `identityProviderMappers` fields, so they are silently dropped on realm updates. To declaratively manage identity providers and their mappers on existing realms (including the `master` realm), use the dedicated CRDs:

- [KeycloakIdentityProvider](./keycloakidentityprovider.md) — manages the identity provider instance.
- [KeycloakIdentityProviderMapper](./keycloakidentityprovidermapper.md) — manages claim, role, and attribute mappers attached to an identity provider.

## Preserving Realm on Deletion

To keep the realm in Keycloak when deleting the CR:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
  annotations:
    keycloak.hostzero.com/preserve-resource: "true"
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
```

See [Common Patterns](../crds.md#preserving-resources-on-deletion) for more details.

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcrm` | `keycloakrealms` |

```bash
kubectl get kcrm
```
