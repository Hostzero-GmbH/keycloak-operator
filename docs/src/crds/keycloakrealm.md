# KeycloakRealm

Manages a Keycloak realm.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instanceRef` | ResourceRef | Yes | Reference to KeycloakInstance |
| `definition` | object | Yes | Realm representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Realm is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `realmId` | string | Keycloak realm ID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app
spec:
  instanceRef:
    name: main
  definition:
    realm: my-app
    enabled: true
    displayName: My Application
    registrationAllowed: false
    loginWithEmailAllowed: true
    duplicateEmailsAllowed: false
    sslRequired: external
    accessTokenLifespan: 300
    ssoSessionIdleTimeout: 1800
```

## Definition Fields

The `definition` field accepts any valid [Keycloak Realm Representation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#RealmRepresentation).

Common fields:

| Field | Type | Description |
|-------|------|-------------|
| `realm` | string | Realm name (required) |
| `enabled` | bool | Whether realm is enabled |
| `displayName` | string | Display name |
| `sslRequired` | string | SSL requirement (none, external, all) |
| `registrationAllowed` | bool | Allow user registration |
| `loginWithEmailAllowed` | bool | Allow login with email |
