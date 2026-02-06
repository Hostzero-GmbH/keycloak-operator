# KeycloakClient

A `KeycloakClient` represents an OAuth2/OIDC client within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
spec:
  # One of realmRef or clusterRealmRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm
    namespace: default  # Optional
  
  # Option 2: Reference to a ClusterKeycloakRealm
  # clusterRealmRef:
  #   name: my-cluster-realm
  
  # Optional: Client ID in Keycloak (defaults to metadata.name)
  clientId: my-app
  
  # Optional: Client definition (Keycloak ClientRepresentation)
  definition:
    clientId: my-app
    name: My Application
    enabled: true
    publicClient: false
    # ... any other Keycloak client properties
  
  # Optional: Configure client secret handling
  clientSecretRef:
    name: my-app-credentials
    # clientIdKey: client-id       # Default: client-id
    # clientSecretKey: client-secret  # Default: client-secret
    # create: true                 # Default: true
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  clientUUID: "12345678-1234-1234-1234-123456789abc"
  resourcePath: "/admin/realms/my-realm/clients/12345678-..."
  message: "Client synchronized successfully"
  instance:
    instanceRef: my-keycloak
  realm:
    realmRef: my-realm
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
```

## Client Secret Handling

The `clientSecretRef` field controls how client secrets are managed:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | (required) | Name of the Kubernetes Secret |
| `clientIdKey` | string | `client-id` | Key for the client ID in the secret |
| `clientSecretKey` | string | `client-secret` | Key for the client secret in the secret |
| `create` | boolean | `true` | Whether to create the secret if it doesn't exist |

### Behavior

- **If the secret exists**: The operator reads the client secret from the specified key and configures Keycloak to use it.
- **If the secret doesn't exist and `create: true`**: The operator lets Keycloak auto-generate a secret and creates the Kubernetes Secret.
- **If the secret doesn't exist and `create: false`**: The operator reports an error (strict mode for GitOps workflows).

### Use Cases

**Auto-generate secret (default):**
```yaml
clientSecretRef:
  name: my-app-credentials
  # create: true (default)
```

**Use pre-existing secret (GitOps/Sealed Secrets):**
```yaml
clientSecretRef:
  name: my-sealed-secret
  create: false
```

**Custom key names:**
```yaml
clientSecretRef:
  name: my-credentials
  clientIdKey: OIDC_CLIENT_ID
  clientSecretKey: OIDC_CLIENT_SECRET
```

## Examples

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
  clientSecretRef:
    name: my-api-credentials
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
  clientSecretRef:
    name: my-service-credentials
```

### Using Pre-existing Secret (Sealed Secrets / External Secrets)

```yaml
# First, create or have your secret management tool create the secret:
apiVersion: v1
kind: Secret
metadata:
  name: my-sealed-secret
type: Opaque
data:
  client-id: bXktYXBw          # base64 encoded
  client-secret: c2VjcmV0...   # base64 encoded
---
# Then reference it with create: false
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-app
    enabled: true
    publicClient: false
  clientSecretRef:
    name: my-sealed-secret
    create: false  # Error if secret doesn't exist
```

## Generated Secret Format

When the operator creates or manages a secret, it has this structure:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-app-credentials
  ownerReferences:
    - apiVersion: keycloak.hostzero.com/v1beta1
      kind: KeycloakClient
      name: my-app
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
