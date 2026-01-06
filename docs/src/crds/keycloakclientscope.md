# KeycloakClientScope

Manages a client scope for token customization.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | ClientScope representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Scope is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `scopeId` | string | Keycloak scope UUID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClientScope
metadata:
  name: custom-claims
spec:
  realmRef:
    name: my-app
  definition:
    name: custom-claims
    protocol: openid-connect
    description: Custom claims for our applications
    attributes:
      include.in.token.scope: "true"
      display.on.consent.screen: "false"
```

## Adding Protocol Mappers

Use [KeycloakProtocolMapper](./keycloakprotocolmapper.md) with `clientScopeRef` to add mappers to scopes.

## Scope Types

| Type | Description |
|------|-------------|
| Default | Automatically included for all clients |
| Optional | Client must explicitly request |
