# KeycloakClient

A `KeycloakClient` represents an OAuth2/OIDC client within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
spec:
  # Required: Reference to the KeycloakRealm
  realmRef:
    name: my-realm
    namespace: default  # Optional
  
  # Optional: Client ID in Keycloak (defaults to metadata.name)
  clientId: my-app
  
  # Required: Client definition (Keycloak ClientRepresentation)
  definition:
    clientId: my-app
    name: My Application
    enabled: true
    publicClient: false
    # ... any other Keycloak client properties
  
  # Optional: Sync client secret to a Kubernetes Secret
  clientSecret:
    secretName: my-app-credentials
    key: clientSecret  # Default: clientSecret
```

## Status

```yaml
status:
  ready: true
  clientId: "my-app"
  clientUUID: "12345678-1234-1234-1234-123456789abc"
  message: "Client synchronized successfully"
```

## Example

### Public Client (SPA)

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-spa
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-spa
    name: My Single Page Application
    enabled: true
    publicClient: true
    standardFlowEnabled: true
    directAccessGrantsEnabled: false
    rootUrl: https://my-app.example.com
    redirectUris:
      - https://my-app.example.com/*
    webOrigins:
      - https://my-app.example.com
```

### Confidential Client (Backend)

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-api
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-api
    name: My Backend API
    enabled: true
    publicClient: false
    serviceAccountsEnabled: true
    standardFlowEnabled: false
    directAccessGrantsEnabled: false
  clientSecret:
    secretName: my-api-credentials
```

### Service Account with Roles

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-service
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-service
    name: My Service Account
    enabled: true
    publicClient: false
    serviceAccountsEnabled: true
    standardFlowEnabled: false
    directAccessGrantsEnabled: false
    authorizationServicesEnabled: true
  clientSecret:
    secretName: my-service-credentials
```

## Client Secret Synchronization

When `clientSecret` is specified, the operator creates a Kubernetes Secret with the client credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-app-credentials
type: Opaque
data:
  client-id: bXktYXBw          # base64 encoded
  client-secret: c2VjcmV0...   # base64 encoded
```

## Definition Properties

Common properties from [Keycloak ClientRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#ClientRepresentation):

| Property | Type | Description |
|----------|------|-------------|
| `clientId` | string | Client identifier (required) |
| `name` | string | Display name |
| `enabled` | boolean | Whether client is enabled |
| `publicClient` | boolean | Public or confidential client |
| `standardFlowEnabled` | boolean | Enable Authorization Code flow |
| `directAccessGrantsEnabled` | boolean | Enable Resource Owner Password flow |
| `serviceAccountsEnabled` | boolean | Enable service account |
| `redirectUris` | string[] | Valid redirect URIs |
| `webOrigins` | string[] | Allowed CORS origins |
| `rootUrl` | string | Root URL for relative URIs |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcc` | `keycloakclients` |

```bash
kubectl get kcc
```
