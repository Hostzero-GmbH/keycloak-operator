# Architecture

This document describes the architecture of the Keycloak Operator.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                                                              │
│  ┌──────────────────┐     ┌──────────────────────────────┐ │
│  │  Keycloak        │     │  Keycloak Operator           │ │
│  │  Instance        │◄────│                              │ │
│  │                  │     │  ┌────────────────────────┐  │ │
│  │  - Realms        │     │  │ Controllers            │  │ │
│  │  - Clients       │     │  │ - Instance Controller  │  │ │
│  │  - Users         │     │  │ - Realm Controller     │  │ │
│  │  - Roles         │     │  │ - Client Controller    │  │ │
│  │  - Groups        │     │  │ - User Controller      │  │ │
│  └──────────────────┘     │  │ - ...                  │  │ │
│                           │  └────────────────────────┘  │ │
│                           │                              │ │
│  ┌──────────────────┐     │  ┌────────────────────────┐  │ │
│  │  Custom          │────►│  │ Keycloak Client        │  │ │
│  │  Resources       │     │  │ - REST API calls       │  │ │
│  │                  │     │  │ - Token management     │  │ │
│  │  - KeycloakRealm │     │  │ - Rate limiting        │  │ │
│  │  - KeycloakClient│     │  └────────────────────────┘  │ │
│  │  - KeycloakUser  │     │                              │ │
│  └──────────────────┘     └──────────────────────────────┘ │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Components

### Controllers

Each CRD has a dedicated controller that watches for changes and reconciles the desired state with Keycloak.

Controllers follow the standard Kubernetes controller pattern:
1. Watch for changes to custom resources
2. Compare desired state with actual state
3. Make API calls to Keycloak to reconcile differences
4. Update resource status

### Keycloak Client

The internal Keycloak client handles all communication with the Keycloak Admin REST API:

- **Authentication**: Obtains and refreshes OAuth2 tokens
- **Rate Limiting**: Prevents overwhelming the Keycloak server
- **Retry Logic**: Handles transient failures gracefully
- **Connection Pooling**: Reuses connections efficiently

### Client Manager

The Client Manager provides shared access to Keycloak clients across controllers:

- Manages a pool of clients per Keycloak instance
- Enforces concurrent request limits
- Provides semaphore-based rate limiting

## Resource Hierarchy

Resources follow a hierarchical dependency model:

```
KeycloakInstance (or ClusterKeycloakInstance)
└── KeycloakRealm (or ClusterKeycloakRealm)
    ├── KeycloakClient
    │   └── KeycloakProtocolMapper
    ├── KeycloakUser
    │   ├── KeycloakRoleMapping
    │   └── KeycloakUserCredential
    ├── KeycloakRole
    ├── KeycloakGroup
    ├── KeycloakClientScope
    │   └── KeycloakProtocolMapper
    ├── KeycloakIdentityProvider
    ├── KeycloakComponent
    └── KeycloakOrganization
```

## Finalizers

All resources use Kubernetes finalizers to ensure proper cleanup:

1. When a resource is deleted, the finalizer prevents immediate removal
2. The controller deletes the corresponding Keycloak resource
3. The finalizer is removed, allowing Kubernetes to complete deletion

## Status Management

Each resource maintains status fields:

- `ready`: Boolean indicating if the resource is synced
- `status`: Human-readable status message
- `message`: Detailed information about the current state
- Resource-specific IDs (e.g., `realmId`, `clientId`)

## Metrics

The operator exposes Prometheus metrics:

- `keycloak_operator_reconcile_total`: Total reconciliations by controller and result
- `keycloak_operator_reconcile_duration_seconds`: Reconciliation duration histogram
- `keycloak_operator_api_requests_total`: Keycloak API requests by method and status
- `keycloak_operator_managed_resources`: Gauge of managed resources by kind
