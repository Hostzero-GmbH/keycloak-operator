# ClusterKeycloakRealm

Cluster-scoped Keycloak realm.

## Overview

`ClusterKeycloakRealm` is identical to `KeycloakRealm` but is cluster-scoped, allowing resources in any namespace to reference it.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instanceRef` | ResourceRef | No* | Reference to KeycloakInstance |
| `clusterInstanceRef` | ResourceRef | No* | Reference to ClusterKeycloakInstance |
| `definition` | object | Yes | Realm representation |

*One of `instanceRef` or `clusterInstanceRef` is required.

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Realm is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `realmId` | string | Keycloak realm ID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakRealm
metadata:
  name: shared-realm
spec:
  clusterInstanceRef:
    name: shared-keycloak
  definition:
    realm: shared
    enabled: true
```

## Usage

Reference from any namespace:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-client
  namespace: my-app
spec:
  clusterRealmRef:
    name: shared-realm
  definition:
    clientId: my-client
    enabled: true
```

## Use Cases

- Shared identity platform across teams
- Multi-tenant applications with shared realm
- Centralized authentication for microservices
