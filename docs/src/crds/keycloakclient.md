# KeycloakClient

> **Identifier field:** Set the client ID in the `spec.clientId` field. It is required and immutable once set. A `clientId` inside `spec.definition` is tolerated only when it matches `spec.clientId`; a conflicting value is rejected.

A `KeycloakClient` represents a client within a Keycloak realm. Both OpenID Connect and SAML clients are supported; the protocol is selected with `definition.protocol` and defaults to `openid-connect`.

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
  
  # Option 2: Reference to a ClusterKeycloakRealm
  # clusterRealmRef:
  #   name: my-cluster-realm
  
  # Required: Client ID in Keycloak (do NOT set clientId in definition)
  clientId: my-app
  
  # Optional: Client definition (Keycloak ClientRepresentation)
  definition:
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
- **Public clients (`publicClient: true`)**: When `clientSecretRef` is set, the Secret is still materialized but only contains the `client-id` key — public OAuth clients have no `client_secret` to store. This lets consumer charts pull the client ID via `envFrom` or `secretKeyRef` regardless of whether the client is public or confidential. If `clientSecretRef` is not set, no Secret is created.

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
  clientId: my-spa
  definition:
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
  # Optional: still materialise a Secret so consumers can mount the
  # client-id via envFrom. The Secret will only contain the client-id
  # key — public clients have no client_secret.
  clientSecretRef:
    name: my-spa-credentials
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
  clientId: my-api
  definition:
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
  clientId: my-service
  definition:
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

### SAML Client

Set `protocol: saml` in the definition. SAML-specific settings are ordinary
`attributes` entries, exactly as in the Keycloak `ClientRepresentation`:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-saml-app
spec:
  realmRef:
    name: my-realm
  clientId: my-saml-app
  definition:
    name: My SAML Application
    enabled: true
    protocol: saml
    redirectUris:
      - "https://app.example.com/saml/*"
    attributes:
      saml_name_id_format: username
      saml.assertion.signature: "true"
      saml.client.signature: "false"
```

Keycloak generates signing material of its own (`saml.signing.certificate`,
`saml.signing.private.key`) and fills in defaults for attributes the definition
omits. The operator leaves those alone: it only compares the properties the
definition declares, and Keycloak merges attributes on update rather than
replacing them.

Claim mappers for a SAML client are [`KeycloakProtocolMapper`](keycloakprotocolmapper.md)
resources with `protocol: saml`.

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
  clientId: my-app
  definition:
    enabled: true
    publicClient: false
  clientSecretRef:
    name: my-sealed-secret
    create: false  # Error if secret doesn't exist
```

## Generated Secret Format

When the operator creates or manages a secret for a **confidential client**, it has this structure:

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

For a **public client** (`publicClient: true`) the Secret is materialized with only the `client-id` key, since there is no OAuth `client_secret`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-spa-credentials
  ownerReferences:
    - apiVersion: keycloak.hostzero.com/v1beta1
      kind: KeycloakClient
      name: my-spa
type: Opaque
data:
  client-id: bXktc3Bh          # base64 encoded
```

## Authentication Flow Binding Overrides

Keycloak allows overriding the default authentication flows (browser, direct grant) per client via `authenticationFlowBindingOverrides`. Normally this requires the internal UUID of the flow, which is generated dynamically and differs across environments -- making it incompatible with GitOps.

The operator supports **alias-based references** so you can use the human-readable flow alias instead:

| Alias Key | Resolves To | Description |
|-----------|-------------|-------------|
| `browserFlowAlias` | `browser` | Browser authentication flow |
| `directGrantFlowAlias` | `direct_grant` | Direct grant (Resource Owner Password) flow |

The operator resolves aliases to UUIDs at reconciliation time. If both an alias key and the corresponding UUID key are present, the alias takes precedence.

### Example: Using flow aliases

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
spec:
  realmRef:
    name: my-realm
  clientId: my-app
  definition:
    enabled: true
    publicClient: true
    standardFlowEnabled: true
    authenticationFlowBindingOverrides:
      browserFlowAlias: "my-custom-browser-flow"
      directGrantFlowAlias: "my-custom-direct-grant"
```

### Example: Using UUIDs (unchanged, still supported)

```yaml
    authenticationFlowBindingOverrides:
      browser: "a3f5c2d1-1234-5678-90ab-abcdef123456"
      direct_grant: "b4e6d3a2-2345-6789-01bc-bcdef2345678"
```

If the specified alias does not match any authentication flow in the realm, the operator reports a `FlowAliasResolutionFailed` status with a descriptive error message.

## Definition Properties

`definition` is the Keycloak [ClientRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#ClientRepresentation),
passed through verbatim. Any property Keycloak accepts can be set here, not just
the ones listed below. Three keys are handled specially: `clientId` belongs in
`spec.clientId`, `defaultClientScopes` and `optionalClientScopes` are applied
through Keycloak's scope-assignment endpoints instead of the client update, and
`protocolMappers` is rejected in favour of
[KeycloakProtocolMapper](keycloakprotocolmapper.md).

Commonly used properties:

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Display name |
| `enabled` | boolean | Whether client is enabled |
| `protocol` | string | `openid-connect` (default) or `saml` |
| `publicClient` | boolean | Public or confidential client |
| `standardFlowEnabled` | boolean | Enable Authorization Code flow |
| `directAccessGrantsEnabled` | boolean | Enable Resource Owner Password flow |
| `serviceAccountsEnabled` | boolean | Enable service account |
| `redirectUris` | string[] | Valid redirect URIs |
| `webOrigins` | string[] | Allowed CORS origins |
| `rootUrl` | string | Root URL for relative URIs |
| `attributes` | map[string]string | Protocol-specific settings, including all `saml.*` options |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcc` | `keycloakclients` |

```bash
kubectl get kcc
```
