# KeycloakComponent

A `KeycloakComponent` manages Keycloak components such as LDAP user federation, custom storage providers, key providers, and other pluggable realm components.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: my-component
spec:
  # One of realmRef or clusterRealmRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm
  
  # Option 2: Reference to a ClusterKeycloakRealm
  # clusterRealmRef:
  #   name: my-cluster-realm
  
  # Required: Component definition
  definition:
    name: corporate-ldap
    providerId: ldap
    providerType: org.keycloak.storage.UserStorageProvider
    config:
      enabled:
        - "true"
      connectionUrl:
        - "ldap://ldap.example.com:389"
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  componentID: "12345678-1234-1234-1234-123456789abc"
  componentName: "corporate-ldap"
  providerType: "org.keycloak.storage.UserStorageProvider"
  message: "Component synchronized successfully"
  resourcePath: "/admin/realms/my-realm/components/12345678-..."
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

### LDAP User Federation

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: ldap-federation
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    name: corporate-ldap
    providerId: ldap
    providerType: org.keycloak.storage.UserStorageProvider
    config:
      enabled:
        - "true"
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
      userObjectClasses:
        - "person, organizationalPerson, user"
      editMode:
        - "READ_ONLY"
```

### RSA Key Provider

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: rsa-key
  namespace: keycloak
spec:
  realmRef:
    name: my-realm
  definition:
    name: rsa-generated
    providerId: rsa-generated
    providerType: org.keycloak.keys.KeyProvider
    config:
      priority:
        - "100"
      algorithm:
        - "RS256"
```

## Definition Properties

The `definition` field accepts any valid Keycloak [ComponentRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#ComponentRepresentation):

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Component name (required) |
| `providerId` | string | Provider ID (e.g., "ldap", "rsa-generated") |
| `providerType` | string | Provider type (e.g., "org.keycloak.storage.UserStorageProvider") |
| `parentId` | string | Parent component ID (defaults to realm ID) |
| `subType` | string | Optional component subtype |
| `config` | object | Provider-specific configuration (array of strings per key) |

## Common Provider Types

| Provider Type | Use Case |
|--------------|----------|
| `org.keycloak.storage.UserStorageProvider` | LDAP, custom user storage |
| `org.keycloak.keys.KeyProvider` | Cryptographic keys (RSA, AES, etc.) |
| `org.keycloak.storage.ldap.mappers.LDAPStorageMapper` | LDAP attribute mappers |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcco` | `keycloakcomponents` |

```bash
kubectl get kcco
```

## Notes

- Component configuration uses arrays of strings for all values
- LDAP credentials should be managed via Kubernetes Secrets (not directly in the CR)
- Some components may require specific ordering via `priority` config
