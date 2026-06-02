# KeycloakIdentityProvider

A `KeycloakIdentityProvider` represents an external identity provider configuration within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: my-idp
spec:
  # One of realmRef or clusterRealmRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm
  
  # Option 2: Reference to a ClusterKeycloakRealm
  clusterRealmRef:
    name: my-cluster-realm
  
  # Optional: Reference to a Secret with config values (e.g. clientId, clientSecret)
  configSecretRef:
    name: my-idp-credentials
  
  # Required: Identity provider definition
  definition:
    alias: my-idp
    providerId: oidc
    enabled: true
    # ... any other properties
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  message: "Identity provider synchronized successfully"
  resourcePath: "/admin/realms/my-realm/identity-provider/instances/my-idp"
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

### OIDC Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: corporate-sso
spec:
  realmRef:
    name: my-realm
  configSecretRef:
    name: corporate-sso-credentials
  definition:
    alias: corporate-sso
    displayName: Corporate SSO
    providerId: oidc
    enabled: true
    trustEmail: true
    firstBrokerLoginFlowAlias: first broker login
    config:
      authorizationUrl: https://sso.corp.example.com/auth
      tokenUrl: https://sso.corp.example.com/token
      userInfoUrl: https://sso.corp.example.com/userinfo
      defaultScope: openid profile email
      syncMode: IMPORT
```

### Google Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: google
spec:
  realmRef:
    name: my-realm
  configSecretRef:
    name: google-idp-credentials
  definition:
    alias: google
    displayName: Sign in with Google
    providerId: google
    enabled: true
    trustEmail: true
    config:
      defaultScope: openid profile email
```

### GitHub Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: github
spec:
  realmRef:
    name: my-realm
  configSecretRef:
    name: github-idp-credentials
  definition:
    alias: github
    displayName: Sign in with GitHub
    providerId: github
    enabled: true
```

### SAML Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: saml-idp
spec:
  realmRef:
    name: my-realm
  definition:
    alias: saml-idp
    displayName: Corporate SAML
    providerId: saml
    enabled: true
    config:
      entityId: https://idp.example.com
      singleSignOnServiceUrl: https://idp.example.com/sso
      nameIDPolicyFormat: urn:oasis:names:tc:SAML:2.0:nameid-format:transient
      signatureAlgorithm: RSA_SHA256
      wantAssertionsSigned: "true"
      wantAuthnRequestsSigned: "true"
```

## Config from Secret

To avoid storing sensitive configuration values (such as `clientId` and `clientSecret`) in plaintext in the CR, use `configSecretRef` to reference a Kubernetes Secret:

```bash
kubectl create secret generic corporate-sso-credentials \
  --from-literal=clientId=my-oidc-client-id \
  --from-literal=clientSecret=my-oidc-client-secret
```

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: corporate-sso
spec:
  realmRef:
    name: my-realm
  configSecretRef:
    name: corporate-sso-credentials
  definition:
    alias: corporate-sso
    providerId: oidc
    enabled: true
    config:
      authorizationUrl: https://sso.corp.example.com/auth
      tokenUrl: https://sso.corp.example.com/token
      defaultScope: openid profile email
```

Every key-value pair in the referenced Secret is merged into `definition.config` before the identity provider is synced to Keycloak. Secret values take precedence over values defined inline in `definition.config`.

The Secret must exist in the same namespace as the `KeycloakIdentityProvider`. When the Secret changes, the operator automatically re-reconciles the identity provider to pick up the new values.

## Token Exchange Permission

When this identity provider should also act as a [Trusted Token Issuer](https://www.keycloak.org/securing-apps/token-exchange) — i.e. clients in the realm exchange a JWT from the upstream IdP for a Keycloak token using RFC 8693 Token Exchange with `subject_issuer=<alias>` — Keycloak needs a fine-grained-authz policy listing which clients are allowed to do so. Without that policy, any client in the realm could perform the exchange.

`spec.tokenExchange` lets the operator manage that policy declaratively. Omit the field to leave Keycloak permissions untouched (default — fully opt-in). Set `allowedClients` to a list of `clientId`s in the same realm:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: external-issuer
spec:
  realmRef:
    name: my-realm
  definition:
    alias: external-issuer
    providerId: oidc
    enabled: true
    config:
      issuer: https://issuer.example.com
      jwksUrl: https://issuer.example.com/.well-known/jwks.json
      useJwksUrl: "true"
      validateSignature: "true"
      hideOnLoginPage: "true"
  tokenExchange:
    allowedClients:
      - my-backend-client
      - my-app-client
```

On reconcile, the operator:

1. Enables fine-grained-authz on the identity provider (PUT `/identity-provider/instances/{alias}/management/permissions`). Keycloak auto-creates a `token-exchange` scope-permission on the `realm-management` authz resource server.
2. Resolves each `allowedClients` entry to its Keycloak client UUID.
3. Ensures a Client-type authz policy named `hostzero-idp-<alias>-token-exchange` exists in `realm-management`'s authz resource server, with `clients` set to the resolved UUIDs.
4. Binds that policy to the auto-created scope-permission. The operator owns the policy list — any other policy bound manually will be replaced.

Any client not on `allowedClients` attempting `subject_issuer=<alias>` is rejected with `403 not_allowed`. An empty `allowedClients: []` is valid and means "deny all".

### Soft wait on referenced clients

On first apply, the identity provider and the clients it references are often siblings in the same wave. If the operator reconciles the identity provider before a referenced client exists in Keycloak, the token-exchange reconcile defers without flipping `status.ready` to false. `status.tokenExchange.message` carries the reason; the next reconcile picks up the client once it lands.

### Status

```yaml
status:
  ready: true
  tokenExchange:
    enabled: true
    permissionID: <UUID of the token-exchange scope-permission>
    policyID:     <UUID of the operator-managed policy>
    policyName:   hostzero-idp-<alias>-token-exchange
```

`enabled: false` with a non-empty `message` indicates the operator is still waiting on referenced state.

### Cleanup

Deleting the identity provider also removes the operator-managed policy from `realm-management`'s authz resource server. The scope-permission itself is removed by Keycloak when management-permissions are toggled off (implicit on IdP delete).

## Definition Properties

Common properties from [Keycloak IdentityProviderRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#IdentityProviderRepresentation):

| Property | Type | Description |
|----------|------|-------------|
| `alias` | string | Unique alias (required) |
| `displayName` | string | Display name |
| `providerId` | string | Provider type (oidc, saml, google, etc.) |
| `enabled` | boolean | Whether provider is enabled |
| `trustEmail` | boolean | Trust email from provider |
| `storeToken` | boolean | Store provider tokens |
| `config` | map | Provider-specific configuration |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcidp` | `keycloakidentityproviders` |

```bash
kubectl get kcidp
```

## Notes

- Use `configSecretRef` to store sensitive values like `clientId` and `clientSecret` in a Kubernetes Secret (see [Config from Secret](#config-from-secret))
- Consider using `syncMode: IMPORT` to import users on first login
- Mappers must be managed via [KeycloakIdentityProviderMapper](./keycloakidentityprovidermapper.md). Embedding `mappers` inside this CR's `definition` is silently ignored by Keycloak on update — the field is only consumed during realm import.
