# Custom Resources

The Keycloak Operator provides 15 custom resource definitions (CRDs) for managing Keycloak configuration.

## Resource Categories

### Core Resources

| Resource | Scope | Description |
|----------|-------|-------------|
| [KeycloakInstance](./crds/keycloakinstance.md) | Namespaced | Connection to a Keycloak server |
| [ClusterKeycloakInstance](./crds/clusterkeycloakinstance.md) | Cluster | Shared Keycloak connection |
| [KeycloakRealm](./crds/keycloakrealm.md) | Namespaced | Keycloak realm |
| [ClusterKeycloakRealm](./crds/clusterkeycloakrealm.md) | Cluster | Shared realm |

### Client Resources

| Resource | Scope | Description |
|----------|-------|-------------|
| [KeycloakClient](./crds/keycloakclient.md) | Namespaced | OAuth2/OIDC client |
| [KeycloakClientScope](./crds/keycloakclientscope.md) | Namespaced | Client scope |
| [KeycloakProtocolMapper](./crds/keycloakprotocolmapper.md) | Namespaced | Token mapper |

### User Resources

| Resource | Scope | Description |
|----------|-------|-------------|
| [KeycloakUser](./crds/keycloakuser.md) | Namespaced | User account |
| [KeycloakUserCredential](./crds/keycloakusercredential.md) | Namespaced | User password |
| [KeycloakRoleMapping](./crds/keycloakrolemapping.md) | Namespaced | Role assignment |

### Authorization Resources

| Resource | Scope | Description |
|----------|-------|-------------|
| [KeycloakRole](./crds/keycloakrole.md) | Namespaced | Realm/client role |
| [KeycloakGroup](./crds/keycloakgroup.md) | Namespaced | User group |

### Integration Resources

| Resource | Scope | Description |
|----------|-------|-------------|
| [KeycloakIdentityProvider](./crds/keycloakidentityprovider.md) | Namespaced | External IdP |
| [KeycloakComponent](./crds/keycloakcomponent.md) | Namespaced | Keycloak component |
| [KeycloakOrganization](./crds/keycloakorganization.md) | Namespaced | Organization (KC 26+) |

## Common Fields

All resources share these common status fields:

```yaml
status:
  ready: true              # Whether resource is synced
  status: "Ready"          # Human-readable status
  message: "Synced"        # Detailed message
```

## Resource References

Resources reference each other using `ResourceRef`:

```yaml
spec:
  instanceRef:           # Reference to KeycloakInstance
    name: main           # Name of the resource
    namespace: default   # Optional, defaults to same namespace
```

## Definition Field

Most resources use a `definition` field containing the Keycloak API representation:

```yaml
spec:
  definition:
    realm: my-realm      # Keycloak API fields
    enabled: true
```

This allows full control over Keycloak configuration while maintaining a simple CRD structure.
