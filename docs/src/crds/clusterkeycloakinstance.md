# ClusterKeycloakInstance

The `ClusterKeycloakInstance` resource makes a Keycloak server known to the operator at the **cluster level**, allowing resources in any namespace to reference it.

## Overview

This is the cluster-scoped equivalent of `KeycloakInstance`. Use it when:
- You have a central Keycloak server shared across multiple namespaces
- You want to avoid duplicating instance definitions in each namespace
- You need cross-namespace realm and client management

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: central-keycloak
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin-credentials
      namespace: keycloak-system
```

### With Client Authentication

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: central-keycloak
spec:
  baseUrl: https://keycloak.example.com
  realm: master
  credentials:
    secretRef:
      name: keycloak-admin-credentials
      namespace: keycloak-system
      usernameKey: admin-user
      passwordKey: admin-password
  client:
    id: admin-cli
```

## Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `baseUrl` | string | URL of the Keycloak server | Yes |
| `credentials.secretRef.name` | string | Name of the credentials secret | Yes |
| `credentials.secretRef.namespace` | string | Namespace of the credentials secret | Yes |
| `credentials.secretRef.usernameKey` | string | Key for username in secret | No (default: "username") |
| `credentials.secretRef.passwordKey` | string | Key for password in secret | No (default: "password") |
| `realm` | string | Admin realm name | No (default: "master") |
| `client.id` | string | Client ID for authentication | No |
| `client.secret` | string | Client secret (if confidential) | No |
| `token.secretName` | string | Secret to cache access tokens | No |
| `token.tokenKey` | string | Key in the secret for the token | No |
| `token.expiresKey` | string | Key in the secret for the token expiration | No |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether connection to Keycloak is established |
| `version` | string | Detected Keycloak server version |
| `status` | string | Current status (Ready, ConnectionFailed, Error) |
| `message` | string | Additional status information |
| `resourcePath` | string | API path for this resource |
| `conditions` | []Condition | Kubernetes conditions |

## Behavior

### Connection Verification

The operator periodically verifies the connection to Keycloak by:
1. Authenticating with the provided credentials
2. Fetching server info to detect the version
3. Updating the `ready` status and connection metrics

### Secret Reference

Since `ClusterKeycloakInstance` is cluster-scoped, the `namespace` field in `secretRef` is **required** (unlike the namespaced `KeycloakInstance` where it defaults to the resource's namespace).

### Client Manager

The operator maintains a pool of authenticated Keycloak clients. When a `ClusterKeycloakInstance` is created, a client is registered in the pool with a special cluster-scoped key, making it available for all resources that reference it.

## Use Cases

### Central Keycloak for Multi-Tenant Platform

```yaml
# Define the central instance once
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: platform-keycloak
spec:
  baseUrl: https://auth.platform.example.com
  credentials:
    secretRef:
      name: keycloak-admin
      namespace: auth-system
---
# Create cluster-scoped realms for each tenant
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: tenant-a-realm
spec:
  clusterInstanceRef:
    name: platform-keycloak
  definition:
    realm: tenant-a
    enabled: true
```

### Shared Instance Across Environments

```yaml
# Credentials in a secure namespace
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-credentials
  namespace: keycloak-secrets
type: Opaque
stringData:
  username: admin
  password: ${KEYCLOAK_ADMIN_PASSWORD}
---
# Cluster instance referencing the secret
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: shared-keycloak
spec:
  baseUrl: https://keycloak.internal.example.com
  credentials:
    secretRef:
      name: keycloak-credentials
      namespace: keycloak-secrets
```

## Comparison with KeycloakInstance

| Aspect | KeycloakInstance | ClusterKeycloakInstance |
|--------|------------------|-------------------------|
| Scope | Namespaced | Cluster |
| Secret namespace | Optional (defaults to same) | Required |
| Accessible from | Same namespace only | Any namespace |
| Short name | `kci` | `ckci` |
| Use case | Single namespace | Multi-namespace/platform |

## Notes

- Only one `ClusterKeycloakInstance` with a given name can exist
- Deleting the instance will invalidate all resources that reference it
- The credentials secret must exist before creating the instance
- The operator requires RBAC permissions to read secrets from the specified namespace
