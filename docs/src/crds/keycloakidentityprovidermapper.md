# KeycloakIdentityProviderMapper

A `KeycloakIdentityProviderMapper` declaratively manages a mapper attached to a `KeycloakIdentityProvider`. Identity provider mappers transform claims, attributes, or roles produced by an external identity provider as users authenticate through it.

This CRD exists because Keycloak's `PUT /admin/realms/{realm}` endpoint silently ignores `identityProviderMappers` (mappers can only be imported with realm creation), and the `IdentityProviderRepresentation` itself has no `mappers` field. The dedicated mapper sub-resource at `/admin/realms/{realm}/identity-provider/instances/{alias}/mappers` is the only API path that allows updating mappers on existing realms (such as the master realm).

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProviderMapper
metadata:
  name: my-mapper
spec:
  # Required: reference to the parent KeycloakIdentityProvider CR.
  # The realm and Keycloak instance are derived from this IdP.
  identityProviderRef:
    name: oidc

  # Required: mapper definition (Keycloak IdentityProviderMapperRepresentation).
  # `identityProviderAlias` is auto-injected from the parent IdP and does not
  # need to be set here.
  definition:
    name: my-mapper
    identityProviderMapper: oidc-role-idp-mapper
    config:
      syncMode: FORCE
      claim: roles
      claim.value: my-group
      role: my-realm-role
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  mapperID: "12345678-1234-1234-1234-123456789abc"
  mapperName: "my-mapper"
  identityProviderAlias: "oidc"
  resourcePath: "/admin/realms/my-realm/identity-provider/instances/oidc/mappers/12345678-..."
  message: "Identity provider mapper synchronized"
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
```

## Examples

### OIDC role mapper

Maps an `roles` claim value of `mdmsupport` (delivered by the upstream IdP) to a Keycloak realm role `mdm-realm.mdm-support`:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProviderMapper
metadata:
  name: mdm-support-role-mapper
  namespace: keycloak
spec:
  identityProviderRef:
    name: oidc
  definition:
    name: mdm-support-role-mapper
    identityProviderMapper: oidc-role-idp-mapper
    config:
      syncMode: FORCE
      claim: roles
      claim.value: mdmsupport
      role: mdm-realm.mdm-support
```

### Hardcoded attribute

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakIdentityProviderMapper
metadata:
  name: oidc-source-attribute
  namespace: keycloak
spec:
  identityProviderRef:
    name: oidc
  definition:
    name: source-attribute
    identityProviderMapper: hardcoded-attribute-idp-mapper
    config:
      syncMode: INHERIT
      attribute: source
      attribute.value: oidc
```

## Parent Reference

| Field | Description |
|-------|-------------|
| `identityProviderRef.name` | Name of the parent `KeycloakIdentityProvider` CR (required) |

The mapper inherits its realm and Keycloak instance from the referenced `KeycloakIdentityProvider`. The mapper's reconciler waits for the parent IdP to reach `Ready` before creating the mapper, and is automatically requeued when the parent transitions to `Ready`.

## Definition Properties

The `definition` field accepts any valid Keycloak [IdentityProviderMapperRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#IdentityProviderMapperRepresentation):

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Mapper name (defaults to `metadata.name` if omitted) |
| `identityProviderMapper` | string | Mapper type (see below) |
| `identityProviderAlias` | string | Auto-injected from the parent IdP; setting it manually is overridden |
| `config` | object | Mapper-specific configuration (all values are strings) |

## Common Identity Provider Mapper Types

| Mapper Type | Description |
|-------------|-------------|
| `oidc-role-idp-mapper` | Grants a Keycloak role when a claim has a specific value |
| `oidc-username-idp-mapper` | Sets the Keycloak username from a claim |
| `oidc-user-attribute-idp-mapper` | Maps a claim to a user attribute |
| `oidc-advanced-role-idp-mapper` | Advanced role mapping with claim conditions |
| `hardcoded-role-idp-mapper` | Always grants a role |
| `hardcoded-attribute-idp-mapper` | Always sets a user attribute |
| `oidc-hardcoded-user-session-attribute-idp-mapper` | Adds a session note |
| `saml-role-idp-mapper` | SAML equivalent of role mapping |
| `saml-user-attribute-idp-mapper` | SAML attribute → user attribute |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcidpm` | `keycloakidentityprovidermappers` |

```bash
kubectl get kcidpm
```

## Notes

- Mapper names must be unique within an identity provider.
- All `config` values are strings (including boolean values like `"true"`/`"false"`).
- The `syncMode` config key controls when the mapper runs: `IMPORT` (only on first login), `FORCE` (every login), or `INHERIT` (use the IdP's own setting).
- Mappers embedded in the `definition` of `KeycloakRealm` or `KeycloakIdentityProvider` are silently dropped by Keycloak on update — always use this CRD to declaratively manage mappers on existing realms.
- Setting the `keycloak.hostzero.com/preserve-resource: "true"` annotation prevents the operator from deleting the mapper in Keycloak when the CR is removed.
