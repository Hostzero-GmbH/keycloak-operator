# KeycloakClient

Manages an OAuth2/OIDC client in a realm.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | Client representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Client is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `clientId` | string | Keycloak client UUID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-frontend
spec:
  realmRef:
    name: my-app
  definition:
    clientId: my-frontend
    enabled: true
    publicClient: true
    standardFlowEnabled: true
    directAccessGrantsEnabled: false
    redirectUris:
      - https://app.example.com/*
    webOrigins:
      - https://app.example.com
```

## Client Types

### Public Client (SPA)

```yaml
definition:
  clientId: spa-app
  publicClient: true
  standardFlowEnabled: true
```

### Confidential Client (Backend)

```yaml
definition:
  clientId: backend-service
  publicClient: false
  serviceAccountsEnabled: true
  clientAuthenticatorType: client-secret
```

### Service Account

```yaml
definition:
  clientId: service-account
  publicClient: false
  serviceAccountsEnabled: true
  standardFlowEnabled: false
  directAccessGrantsEnabled: false
```
