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
  alias: "my-idp"
  message: "Identity provider synchronized successfully"
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
      clientId: keycloak-client
      clientSecret: client-secret-here
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
  definition:
    alias: google
    displayName: Sign in with Google
    providerId: google
    enabled: true
    trustEmail: true
    config:
      clientId: your-google-client-id
      clientSecret: your-google-client-secret
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
  definition:
    alias: github
    displayName: Sign in with GitHub
    providerId: github
    enabled: true
    config:
      clientId: your-github-client-id
      clientSecret: your-github-client-secret
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

- Store sensitive values like client secrets in Kubernetes Secrets and reference them
- Consider using `syncMode: IMPORT` to import users on first login
- Configure mappers to transform claims from the external provider
