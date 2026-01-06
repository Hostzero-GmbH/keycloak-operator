# KeycloakIdentityProvider

Manages external identity provider configuration.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | IdP representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | IdP is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `alias` | string | IdP alias |

## Example - Google

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: google-idp
spec:
  realmRef:
    name: my-app
  definition:
    alias: google
    providerId: google
    enabled: true
    trustEmail: true
    config:
      clientId: your-google-client-id
      clientSecret: your-google-client-secret
```

## Example - OIDC

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProvider
metadata:
  name: corporate-sso
spec:
  realmRef:
    name: my-app
  definition:
    alias: corporate
    providerId: oidc
    enabled: true
    config:
      authorizationUrl: https://sso.corp.com/authorize
      tokenUrl: https://sso.corp.com/token
      clientId: keycloak-client
      clientSecret: secret
```

## Supported Providers

- google, facebook, github, gitlab
- oidc (generic OpenID Connect)
- saml (SAML 2.0)
- ldap (via KeycloakComponent)
