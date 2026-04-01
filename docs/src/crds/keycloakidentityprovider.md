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
    namespace: default  # Optional, defaults to same namespace
  
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
- Configure mappers to transform claims from the external provider
