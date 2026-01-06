# ClusterKeycloakInstance

Cluster-scoped Keycloak instance connection.

## Overview

`ClusterKeycloakInstance` is identical to `KeycloakInstance` but is cluster-scoped, allowing it to be referenced from any namespace.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `baseUrl` | string | Yes | Base URL of the Keycloak server |
| `credentials` | CredentialsSpec | Yes | Admin credentials |

### CredentialsSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secretRef.name` | string | Yes | Name of the secret |
| `secretRef.namespace` | string | Yes | Namespace of the secret |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Connection is established |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `version` | string | Keycloak server version |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: shared-keycloak
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin
      namespace: keycloak-system
```

## Usage

Reference from any namespace:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
  namespace: my-app
spec:
  clusterInstanceRef:
    name: shared-keycloak
  definition:
    realm: my-realm
    enabled: true
```
