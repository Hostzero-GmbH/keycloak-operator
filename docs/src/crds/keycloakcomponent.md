# KeycloakComponent

Manages Keycloak components like key providers, LDAP, etc.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | Component representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Component is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `componentId` | string | Keycloak component UUID |

## Example - RSA Key Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: rsa-key
spec:
  realmRef:
    name: my-app
  definition:
    name: rsa-generated
    providerId: rsa-generated
    providerType: org.keycloak.keys.KeyProvider
    config:
      priority:
        - "100"
      keySize:
        - "2048"
```

## Example - LDAP User Federation

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: ldap-federation
spec:
  realmRef:
    name: my-app
  definition:
    name: ldap
    providerId: ldap
    providerType: org.keycloak.storage.UserStorageProvider
    config:
      vendor:
        - "ad"
      connectionUrl:
        - "ldap://ldap.example.com:389"
      bindDn:
        - "cn=admin,dc=example,dc=com"
      bindCredential:
        - "secret"
      usersDn:
        - "ou=users,dc=example,dc=com"
```

## Component Types

| Provider Type | Description |
|--------------|-------------|
| `org.keycloak.keys.KeyProvider` | Cryptographic keys |
| `org.keycloak.storage.UserStorageProvider` | User federation (LDAP) |
