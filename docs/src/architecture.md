# Architecture

The Keycloak Operator follows the Kubernetes operator pattern to manage Keycloak resources declaratively.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      Kubernetes Cluster                          │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │                    Custom Resources                           ││
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐               ││
│  │  │ Keycloak   │ │ Keycloak   │ │ Keycloak   │               ││
│  │  │ Instance   │ │ Realm      │ │ Client     │  ...          ││
│  │  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘               ││
│  └────────┼──────────────┼──────────────┼───────────────────────┘│
│           │              │              │                        │
│           ▼              ▼              ▼                        │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │                   Keycloak Operator                           ││
│  │  ┌────────────────────────────────────────────────────────┐  ││
│  │  │                  Controller Manager                     │  ││
│  │  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐   │  ││
│  │  │  │   Instance   │ │    Realm     │ │    Client    │   │  ││
│  │  │  │  Controller  │ │  Controller  │ │  Controller  │   │  ││
│  │  │  └──────────────┘ └──────────────┘ └──────────────┘   │  ││
│  │  └────────────────────────────────────────────────────────┘  ││
│  │                            │                                  ││
│  │                            ▼                                  ││
│  │  ┌────────────────────────────────────────────────────────┐  ││
│  │  │                  Keycloak Client                        │  ││
│  │  │         (custom resty-based HTTP client)                │  ││
│  │  └────────────────────────────────────────────────────────┘  ││
│  └──────────────────────────────────────────────────────────────┘│
│                               │                                  │
└───────────────────────────────┼──────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │    Keycloak Server    │
                    │   (Admin REST API)    │
                    └───────────────────────┘
```

## Components

### Controller Manager

The controller manager is the main component that runs all controllers. It is built using the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library.

Key features:
- Leader election for high availability
- Health and readiness probes
- Metrics endpoint for Prometheus
- Graceful shutdown handling

### Controllers

Each CRD type has a dedicated controller that implements the reconciliation logic:

| Controller | CRD | Responsibilities |
|------------|-----|------------------|
| Instance Controller | KeycloakInstance, ClusterKeycloakInstance | Connection management, health checking |
| Realm Controller | KeycloakRealm, ClusterKeycloakRealm | Realm CRUD, configuration sync |
| Client Controller | KeycloakClient | Client CRUD, secret management |
| ClientScope Controller | KeycloakClientScope | Scope CRUD |
| ProtocolMapper Controller | KeycloakProtocolMapper | Token claim mapper configuration |
| User Controller | KeycloakUser | User CRUD |
| UserCredential Controller | KeycloakUserCredential | Password management |
| Group Controller | KeycloakGroup | Group CRUD, hierarchy management |
| Role Controller | KeycloakRole | Realm and client role management |
| RoleMapping Controller | KeycloakRoleMapping | Role-to-subject assignments |
| IdentityProvider Controller | KeycloakIdentityProvider | External IDP configuration |
| Component Controller | KeycloakComponent | LDAP, key providers, etc. |
| Organization Controller | KeycloakOrganization | Organization management (KC 26+) |

### Keycloak Client

The operator uses a custom HTTP client built on [resty](https://github.com/go-resty/resty). Key features:

- **Version-agnostic**: Works with raw JSON to support all Keycloak versions
- **Connection pooling**: Multiple KeycloakInstances share clients via `ClientManager`
- **Token management**: Automatic token acquisition and refresh
- **Retry logic**: Exponential backoff for transient errors (5xx, network issues)
- **Pass-through definitions**: CR definitions are sent directly to Keycloak without field stripping

## Reconciliation Flow

```
                    ┌─────────────────┐
                    │  CR Created/    │
                    │  Updated/Deleted│
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │   Controller    │
                    │   Triggered     │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Get Current    │
                    │  State from KC  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ Compare Desired │
                    │ vs Actual State │
                    └────────┬────────┘
                             │
            ┌────────────────┼────────────────┐
            ▼                ▼                ▼
      ┌──────────┐    ┌──────────┐    ┌──────────┐
      │  Create  │    │  Update  │    │  Delete  │
      │ in KC    │    │ in KC    │    │ from KC  │
      └────┬─────┘    └────┬─────┘    └────┬─────┘
           │               │               │
           └───────────────┴───────────────┘
                           │
                           ▼
                    ┌─────────────────┐
                    │  Update CR      │
                    │  Status         │
                    └─────────────────┘
```

## Resource Dependencies

Resources form a hierarchy with parent-child relationships:

```
KeycloakInstance (connection to Keycloak)
│
└── KeycloakRealm (realm within instance)
    │
    ├── KeycloakClient (client within realm)
    │
    ├── KeycloakUser (user within realm)
    │
    ├── KeycloakGroup (group within realm)
    │   │
    │   └── KeycloakGroup (nested child group)
    │
    ├── KeycloakClientScope (scope within realm)
    │
    └── KeycloakIdentityProvider (IDP within realm)
```

Controllers resolve parent references and wait for parents to be ready before proceeding.

## Finalizers

The operator uses finalizers to ensure proper cleanup:

1. When a CR is created, a finalizer is added
2. When a CR is deleted, the controller:
   - Removes the resource from Keycloak
   - Removes the finalizer
3. Kubernetes then removes the CR

This ensures resources are properly cleaned up in Keycloak even if the cluster is disrupted.

## High Availability

The operator supports running multiple replicas with leader election:

- Only the leader processes reconciliations
- Other replicas are hot standby
- Automatic failover on leader failure
- Configurable via `leaderElection.enabled` Helm value

## Performance Tuning

For large deployments with many resources, the operator provides tuning options:

### Sync Period

The `--sync-period` flag controls how often successfully reconciled resources are re-checked for drift:

```bash
# Default: 5 minutes
--sync-period=5m

# For large deployments (100+ resources): 30 minutes
--sync-period=30m

# For very large deployments or slow networks: 1 hour
--sync-period=1h
```

**Trade-offs:**
- **Shorter periods**: Faster drift detection, higher Keycloak API load
- **Longer periods**: Lower API load, slower drift detection

In Helm:
```yaml
performance:
  syncPeriod: "30m"
```

### Rate Limiting

The `--max-concurrent-requests` flag limits parallel requests to Keycloak:

```bash
# Default: 10 concurrent requests
--max-concurrent-requests=10

# For resource-constrained Keycloak instances
--max-concurrent-requests=5

# No limit (not recommended for large deployments)
--max-concurrent-requests=0
```

**Trade-offs:**
- **Lower values**: Less Keycloak load, slower reconciliation on startup
- **Higher values**: Faster reconciliation, more Keycloak load

In Helm:
```yaml
performance:
  maxConcurrentRequests: 5
```

### Recommendations by Scale

| Resources | Sync Period | Max Concurrent Requests |
|-----------|-------------|------------------------|
| < 50 | 5m (default) | 10 (default) |
| 50-200 | 15-30m | 10 |
| 200-500 | 30m | 5-10 |
| 500+ | 1h | 5 |

The exact values depend on your Keycloak instance capacity and acceptable drift detection latency.
