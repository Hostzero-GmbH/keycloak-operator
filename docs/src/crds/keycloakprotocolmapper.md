# KeycloakProtocolMapper

A `KeycloakProtocolMapper` defines how user attributes, roles, and other data are mapped into tokens. Protocol mappers can be attached to either clients or client scopes.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakProtocolMapper
metadata:
  name: my-mapper
spec:
  # One of clientRef or clientScopeRef must be specified
  clientRef:
    name: my-client
  
  # Or for client scopes:
  # clientScopeRef:
  #   name: my-scope
  
  # Required: Mapper definition
  definition:
    name: department
    protocol: openid-connect
    protocolMapper: oidc-usermodel-attribute-mapper
    config:
      user.attribute: department
      claim.name: department
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  mapperID: "12345678-1234-1234-1234-123456789abc"
  mapperName: "department"
  parentType: "client"
  parentID: "87654321-..."
  message: "Protocol mapper synchronized successfully"
  resourcePath: "/admin/realms/my-realm/clients/87654321-.../protocol-mappers/models/12345678-..."
  instance:
    instanceRef: my-keycloak
  realm:
    realmRef: my-realm
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
```

## Examples

### Client Protocol Mapper

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakProtocolMapper
metadata:
  name: department-mapper
  namespace: keycloak
spec:
  clientRef:
    name: my-client
  definition:
    name: department
    protocol: openid-connect
    protocolMapper: oidc-usermodel-attribute-mapper
    config:
      user.attribute: department
      claim.name: department
      jsonType.label: String
      id.token.claim: "true"
      access.token.claim: "true"
      userinfo.token.claim: "true"
```

### Client Scope Protocol Mapper

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakProtocolMapper
metadata:
  name: groups-mapper
  namespace: keycloak
spec:
  clientScopeRef:
    name: my-scope
  definition:
    name: groups
    protocol: openid-connect
    protocolMapper: oidc-group-membership-mapper
    config:
      full.path: "false"
      id.token.claim: "true"
      access.token.claim: "true"
      claim.name: groups
      userinfo.token.claim: "true"
```

## Parent Reference

A `KeycloakProtocolMapper` belongs to either a client or client scope:

| Reference | Use Case |
|-----------|----------|
| `clientRef` | Mapper applies to a specific client only |
| `clientScopeRef` | Mapper applies to all clients using the scope |

**Note:** Exactly one of these must be specified.

## Definition Properties

The `definition` field accepts any valid Keycloak [ProtocolMapperRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#ProtocolMapperRepresentation):

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Mapper name (required) |
| `protocol` | string | Protocol (usually "openid-connect" or "saml") |
| `protocolMapper` | string | Mapper type (see common types below) |
| `consentRequired` | boolean | Whether user consent is required |
| `config` | object | Mapper-specific configuration |

## Common Protocol Mapper Types

### OpenID Connect

| Mapper Type | Description |
|-------------|-------------|
| `oidc-usermodel-attribute-mapper` | Maps user attribute to token claim |
| `oidc-usermodel-property-mapper` | Maps user property to token claim |
| `oidc-group-membership-mapper` | Includes group membership in token |
| `oidc-role-name-mapper` | Maps role names |
| `oidc-hardcoded-claim-mapper` | Adds hardcoded claim |
| `oidc-audience-mapper` | Adds audience to token |
| `oidc-full-name-mapper` | Maps full name |

### SAML

| Mapper Type | Description |
|-------------|-------------|
| `saml-user-attribute-mapper` | Maps user attribute |
| `saml-group-membership-mapper` | Maps group membership |
| `saml-role-list-mapper` | Maps roles |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcpm` | `keycloakprotocolmappers` |

```bash
kubectl get kcpm
```

## Notes

- Mapper names must be unique within the client or client scope
- The `config` values are all strings (including boolean values like "true"/"false")
- Changes to mappers affect all tokens issued after the change
