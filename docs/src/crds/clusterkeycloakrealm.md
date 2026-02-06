# ClusterKeycloakRealm

The `ClusterKeycloakRealm` resource defines a Keycloak realm at the **cluster level**, making it accessible to resources in any namespace.

## Overview

This is the cluster-scoped equivalent of `KeycloakRealm`. Use it when:
- You need a realm that can be referenced from multiple namespaces
- You're using `ClusterKeycloakInstance` for your Keycloak server
- You want centralized realm management with distributed client/user definitions

## Examples

### Basic Cluster Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: shared-realm
spec:
  clusterInstanceRef:
    name: central-keycloak
  definition:
    realm: shared
    enabled: true
    displayName: Shared Platform Realm
```

### With Namespaced Instance

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: company-realm
spec:
  instanceRef:
    name: keycloak-instance
    namespace: keycloak-system
  definition:
    realm: company
    enabled: true
    loginWithEmailAllowed: true
    registrationAllowed: false
```

### Full Configuration

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: production-realm
spec:
  clusterInstanceRef:
    name: production-keycloak
  realmName: prod  # Override the Keycloak realm name
  definition:
    realm: prod
    enabled: true
    displayName: Production
    sslRequired: external
    registrationAllowed: false
    loginWithEmailAllowed: true
    duplicateEmailsAllowed: false
    resetPasswordAllowed: true
    verifyEmail: true
    bruteForceProtected: true
    accessTokenLifespan: 300
    ssoSessionIdleTimeout: 1800
    ssoSessionMaxLifespan: 36000
    loginTheme: keycloak
    accountTheme: keycloak
    adminTheme: keycloak
    emailTheme: keycloak
```

## Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `clusterInstanceRef.name` | string | Reference to ClusterKeycloakInstance | One of these |
| `instanceRef.name` | string | Reference to namespaced KeycloakInstance | One of these |
| `instanceRef.namespace` | string | Namespace of the KeycloakInstance | Required if instanceRef |
| `realmName` | string | Override realm name in Keycloak | No (defaults to metadata.name) |
| `definition` | object | Keycloak RealmRepresentation | Yes |

### Definition Fields

The `definition` field accepts any valid Keycloak RealmRepresentation. Common fields include:

| Field | Type | Description |
|-------|------|-------------|
| `realm` | string | Realm identifier (required) |
| `enabled` | boolean | Whether realm is enabled |
| `displayName` | string | Display name |
| `sslRequired` | string | SSL requirement: all, external, none |
| `registrationAllowed` | boolean | Allow user self-registration |
| `loginWithEmailAllowed` | boolean | Allow login with email |
| `verifyEmail` | boolean | Require email verification |
| `resetPasswordAllowed` | boolean | Enable password reset |
| `bruteForceProtected` | boolean | Enable brute force protection |
| `accessTokenLifespan` | integer | Access token lifetime (seconds) |
| `ssoSessionIdleTimeout` | integer | SSO session idle timeout (seconds) |
| `loginTheme` | string | Login page theme |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the realm is synced |
| `status` | string | Current status (Ready, InstanceNotReady, CreateFailed, etc.) |
| `message` | string | Additional status information |
| `resourcePath` | string | Keycloak API path for this realm |
| `realmName` | string | Actual realm name in Keycloak |
| `instance` | object | Resolved instance reference |
| `conditions` | []Condition | Kubernetes conditions |

## Behavior

### Instance Resolution

The controller supports two instance reference types:

1. **clusterInstanceRef**: References a `ClusterKeycloakInstance` by name
2. **instanceRef**: References a namespaced `KeycloakInstance` by name and namespace

One of these must be specified.

### Realm Synchronization

On each reconciliation:
1. Connect to Keycloak using the referenced instance
2. Check if the realm exists
3. Create or update the realm with the specified definition
4. Update status with the resource path

### Cleanup

When a `ClusterKeycloakRealm` is deleted:
1. The finalizer removes the realm from Keycloak
2. All resources in Keycloak (clients, users, etc.) within that realm are deleted
3. The Kubernetes resource is then removed

## Use Cases

### Multi-Tenant Platform

```yaml
# Central Keycloak instance
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: platform-keycloak
spec:
  baseUrl: https://auth.example.com
  credentials:
    secretRef:
      name: admin-creds
      namespace: auth-system
---
# Realm for each tenant (cluster-scoped)
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: tenant-acme
spec:
  clusterInstanceRef:
    name: platform-keycloak
  definition:
    realm: acme
    enabled: true
    displayName: ACME Corporation
---
# Clients can be in any namespace
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: acme-web-app
  namespace: acme-apps
spec:
  clusterRealmRef:
    name: tenant-acme
  definition:
    clientId: acme-web-app
    protocol: openid-connect
    publicClient: true
    redirectUris:
      - https://app.acme.example.com/*
```

### Environment-Specific Realms

```yaml
# Development realm
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: app-dev
spec:
  clusterInstanceRef:
    name: keycloak-dev
  definition:
    realm: app-dev
    enabled: true
    registrationAllowed: true  # Allow in dev
    sslRequired: none  # Relaxed for dev
---
# Production realm
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: app-prod
spec:
  clusterInstanceRef:
    name: keycloak-prod
  definition:
    realm: app-prod
    enabled: true
    registrationAllowed: false
    sslRequired: external
    bruteForceProtected: true
    verifyEmail: true
```

## Referencing from Namespaced Resources

Resources in any namespace can reference a `ClusterKeycloakRealm`:

```yaml
# KeycloakClient in namespace-a
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-client
  namespace: namespace-a
spec:
  clusterRealmRef:
    name: shared-realm  # References ClusterKeycloakRealm
  definition:
    clientId: my-client
    # ...
---
# KeycloakUser in namespace-b
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakUser
metadata:
  name: my-user
  namespace: namespace-b
spec:
  clusterRealmRef:
    name: shared-realm  # Same ClusterKeycloakRealm
  definition:
    username: myuser
    # ...
```

## Comparison with KeycloakRealm

| Aspect | KeycloakRealm | ClusterKeycloakRealm |
|--------|---------------|----------------------|
| Scope | Namespaced | Cluster |
| Instance ref | Same namespace or cross-namespace | Cluster or any namespaced |
| Accessible from | Same namespace | Any namespace |
| Short name | `kcrm` | `ckcrm` |
| Use case | Single namespace | Multi-namespace/platform |

## Notes

- Only one `ClusterKeycloakRealm` with a given name can exist
- Deleting the realm will delete all Keycloak resources within it
- The referenced instance must be ready before the realm can be created
- Changes to the `definition` are applied on each reconciliation
