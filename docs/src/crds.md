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
            │       └── KeycloakIdentityProviderMapper
            ├── KeycloakAuthenticationFlow
            ├── KeycloakRequiredAction
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
| [KeycloakIdentityProviderMapper](./crds/keycloakidentityprovidermapper.md) | Identity provider claim/role/attribute mappers | KeycloakIdentityProvider |
| [KeycloakAuthenticationFlow](./crds/keycloakauthenticationflow.md) | Custom authentication / registration flows | KeycloakRealm |
| [KeycloakRequiredAction](./crds/keycloakrequiredaction.md) | Required action providers (e.g. update password, verify email) | KeycloakRealm |
| [KeycloakOrganization](./crds/keycloakorganization.md) | Organization management² | KeycloakRealm |

¹ KeycloakUser supports `clientRef` for managing service account users associated with a client  
² KeycloakOrganization requires Keycloak 26.0.0 or later

## Common Patterns

### Spec Layout

Every CRD that mirrors a Keycloak representation is built from the same four layers:

1. **Placement refs** — typed references that anchor the resource in the hierarchy: `realmRef`, `clusterRealmRef`, `clientRef`, `clientScopeRef`, `parentGroupRef`, `identityProviderRef`, …
2. **Identity** — one typed, required, immutable field naming the object in Keycloak: `realmName`, `clientId`, `username`, `alias`, or `name`. It is mirrored to status and shown as a printer column.
3. **Kubernetes integration** — typed fields for anything that touches cluster objects or other CRs: Secret references (`clientSecretRef`, `smtpSecretRef`, `configSecretRef`), `initialPassword`, `tokenExchange`. The operator injects these into the payload or applies them through separate Keycloak API calls.
4. **Payload** — `spec.definition`, the verbatim Keycloak API representation. The operator writes into it (identifier, secret merges) but never reads configuration out of it; everything else passes through unchanged. This is what lets you configure any Keycloak property, even ones the CRD does not model.

```yaml
spec:
  instanceRef:                  # 1. placement
    name: my-keycloak
  realmName: my-realm           # 2. identity
  smtpSecretRef:                # 3. Kubernetes integration
    name: smtp-credentials
  definition:                   # 4. payload — full Keycloak API object
    enabled: true
    displayName: My Realm
```

**Every datum has exactly one home.** A value that belongs to a typed layer must not also be configured inside `definition`: an identifier in the definition is tolerated only when it matches the spec field, and a conflicting value sets `Ready=False`.

Fully typed CRDs without a `definition` (KeycloakInstance, KeycloakRoleMapping, KeycloakUserCredential) are operator constructs rather than representation mirrors, so they have no payload layer.

For contributors, the layer of a new field follows mechanically:

- References a Kubernetes object or another CR? → typed spec field (layer 3).
- Managed through a separate Keycloak API endpoint rather than the representation `PUT`? → its own CRD (like KeycloakRoleMapping) or a typed spec section — never keys inside `definition`.
- Part of the Keycloak representation itself? → stays in `definition`, untyped.

### Placement References

**A resource names exactly one parent, and if that parent implies the realm, the realm is not named again.** Setting more than one, or none, is rejected at admission.

| CRD | Placement refs |
|-----|----------------|
| `KeycloakRealm`, `ClusterKeycloakRealm` | `instanceRef` / `clusterInstanceRef` |
| `KeycloakClient`, `KeycloakClientScope`, `KeycloakComponent`, `KeycloakOrganization`, `KeycloakIdentityProvider`, `KeycloakRequiredAction`, `KeycloakAuthenticationFlow` | `realmRef` / `clusterRealmRef` |
| `KeycloakRole`, `KeycloakUser` | `realmRef` / `clusterRealmRef` / `clientRef` |
| `KeycloakGroup` | `realmRef` / `clusterRealmRef` / `parentGroupRef` |
| `KeycloakProtocolMapper` | `clientRef` / `clientScopeRef` |
| `KeycloakRoleMapping` | `subject.userRef` / `subject.groupRef` / `subject.serviceAccountRef` |
| `KeycloakIdentityProviderMapper` | `identityProviderRef` |
| `KeycloakUserCredential` | `userRef` |

Where a ref points at something below the realm, the realm is derived from it: a client role reads the realm of its `clientRef`, and a nested group inherits the realm carried by the root of its `parentGroupRef` chain. Restating it alongside would allow the two to disagree, which is why it is rejected rather than merely redundant.

Secret and ConfigMap references (`clientSecretRef`, `smtpSecretRef`, `configSecretRef`, …) are layer 3, not placement, and are unaffected by this rule.

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

### Preserving Resources on Deletion

By default, when you delete a Custom Resource, the operator also deletes the corresponding resource in Keycloak. If you want to keep the resource in Keycloak while removing the CR from Kubernetes, use the `keycloak.hostzero.com/preserve-resource` annotation:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
  annotations:
    keycloak.hostzero.com/preserve-resource: "true"
spec:
  # ...
```

When this annotation is set to `"true"`, deleting the CR will:
- Remove the CR from Kubernetes
- **Keep** the resource in Keycloak untouched

This is useful for scenarios like:
- Migrating management of a resource to a different system
- Temporarily removing operator control without losing data
- Testing or debugging without affecting production resources

> **Note**: The annotation value must be exactly `"true"` (as a string) to preserve the resource. Any other value (or absence of the annotation) will result in normal deletion behavior.

**Supported Resources**: This annotation works with all resource types except `KeycloakInstance` and `ClusterKeycloakInstance` (which don't manage Keycloak resources directly).

## API Version

All CRDs use the `keycloak.hostzero.com/v1beta1` API version:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
```
