# KeycloakProtocolMapper

Manages protocol mappers for token customization.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clientRef` | ResourceRef | No* | Reference to KeycloakClient |
| `clientScopeRef` | ResourceRef | No* | Reference to KeycloakClientScope |
| `definition` | object | Yes | Mapper representation |

*One of `clientRef` or `clientScopeRef` is required.

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Mapper is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `mapperId` | string | Keycloak mapper UUID |

## Example - User Attribute Mapper

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakProtocolMapper
metadata:
  name: department-mapper
spec:
  clientRef:
    name: my-frontend
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

## Common Mapper Types

| Type | Description |
|------|-------------|
| `oidc-usermodel-attribute-mapper` | Map user attribute to claim |
| `oidc-usermodel-property-mapper` | Map user property to claim |
| `oidc-hardcoded-claim-mapper` | Add hardcoded claim |
| `oidc-audience-mapper` | Add audience to token |
| `oidc-group-membership-mapper` | Add group memberships |
