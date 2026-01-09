# Custom Resource Definitions

The Keycloak Operator provides several Custom Resource Definitions (CRDs) to manage Keycloak resources declaratively.

## Resource Hierarchy

```
KeycloakInstance / ClusterKeycloakInstance
    └── KeycloakRealm / ClusterKeycloakRealm
            ├── KeycloakClient
            │       ├── KeycloakUser (service account, via clientRef)
            │       ├── KeycloakRole (client role)
            │       └── KeycloakProtocolMapper
            ├── KeycloakUser (regular users, via realmRef)
            │       └── KeycloakUserCredential
            ├── KeycloakGroup
            ├── KeycloakClientScope
            │       └── KeycloakProtocolMapper
            ├── KeycloakRole (realm role)
            ├── KeycloakRoleMapping (maps roles to Users/Groups)
            ├── KeycloakComponent (LDAP, key providers, etc.)
            ├── KeycloakIdentityProvider
            └── KeycloakOrganization (requires Keycloak 26+)
```

## Overview

### Instance Resources

| CRD | Description | Scope |
|-----|-------------|-------|
| [KeycloakInstance](./crds/keycloakinstance.md) | Connection to a Keycloak server | Namespaced |
| [ClusterKeycloakInstance](./crds/clusterkeycloakinstance.md) | Cluster-scoped Keycloak connection | Cluster |

### Realm Resources

| CRD | Description | Parent |
|-----|-------------|--------|
| [KeycloakRealm](./crds/keycloakrealm.md) | Realm configuration | KeycloakInstance |
| [ClusterKeycloakRealm](./crds/clusterkeycloakrealm.md) | Cluster-scoped realm | ClusterKeycloakInstance |

### OAuth & Client Resources

| CRD | Description | Parent |
|-----|-------------|--------|
| [KeycloakClient](./crds/keycloakclient.md) | OAuth2/OIDC client | KeycloakRealm |
| [KeycloakClientScope](./crds/keycloakclientscope.md) | Client scope configuration | KeycloakRealm |
| [KeycloakProtocolMapper](./crds/keycloakprotocolmapper.md) | Token claim mappers | KeycloakClient or KeycloakClientScope |

### Identity Resources

| CRD | Description | Parent |
|-----|-------------|--------|
| [KeycloakUser](./crds/keycloakuser.md) | User management | KeycloakRealm or KeycloakClient¹ |
| [KeycloakUserCredential](./crds/keycloakusercredential.md) | User password management | KeycloakUser |
| [KeycloakGroup](./crds/keycloakgroup.md) | Group management | KeycloakRealm |

### Role & Access Control

| CRD | Description | Parent |
|-----|-------------|--------|
| [KeycloakRole](./crds/keycloakrole.md) | Realm and client roles | KeycloakRealm or KeycloakClient |
| [KeycloakRoleMapping](./crds/keycloakrolemapping.md) | Role-to-subject mappings | KeycloakUser or KeycloakGroup |

### Federation & Infrastructure

| CRD | Description | Parent |
|-----|-------------|--------|
| [KeycloakComponent](./crds/keycloakcomponent.md) | LDAP federation, key providers | KeycloakRealm |
| [KeycloakIdentityProvider](./crds/keycloakidentityprovider.md) | External identity providers | KeycloakRealm |
| [KeycloakOrganization](./crds/keycloakorganization.md) | Organization management² | KeycloakRealm |

¹ KeycloakUser supports `clientRef` for managing service account users associated with a client  
² KeycloakOrganization requires Keycloak 26.0.0 or later

## Common Patterns

### Definition Field

Most resources include a `definition` field that accepts the full Keycloak API representation:

```yaml
spec:
  definition:
    # Full Keycloak API object
    realm: my-realm
    enabled: true
    displayName: My Realm
```

This provides flexibility to configure any Keycloak property, even those not explicitly modeled in the CRD.

### Status Tracking

All resources expose status information:

```yaml
status:
  ready: true
  message: "Resource synchronized successfully"
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2024-01-01T00:00:00Z"
      reason: Synchronized
      message: "Resource is in sync with Keycloak"
```

### Finalizers

Resources use finalizers to ensure proper cleanup when deleted:

```yaml
metadata:
  finalizers:
    - keycloak.hostzero.com/finalizer
```

## API Version

All CRDs use the `keycloak.hostzero.com/v1beta1` API version:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
```
