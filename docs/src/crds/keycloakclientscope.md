# KeycloakClientScope

A `KeycloakClientScope` represents a client scope within a Keycloak realm.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClientScope
metadata:
  name: my-scope
spec:
  # One of realmRef or clusterRealmRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakRealm
  realmRef:
    name: my-realm
    namespace: default  # Optional, defaults to same namespace
  
  # Option 2: Reference to a ClusterKeycloakRealm
  clusterRealmRef:
    name: my-cluster-realm
  
  # Required: Client scope definition
  definition:
    name: my-scope
    protocol: openid-connect
    # ... any other properties
```

## Status

```yaml
status:
  ready: true
  scopeId: "12345678-1234-1234-1234-123456789abc"
  message: "Client scope synchronized successfully"
```

## Example

### Basic Scope

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClientScope
metadata:
  name: profile-extended
spec:
  realmRef:
    name: my-realm
  definition:
    name: profile-extended
    description: Extended profile information
    protocol: openid-connect
```

### Scope with Protocol Mappers

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClientScope
metadata:
  name: department-scope
spec:
  realmRef:
    name: my-realm
  definition:
    name: department
    description: Department information
    protocol: openid-connect
    protocolMappers:
      - name: department
        protocol: openid-connect
        protocolMapper: oidc-usermodel-attribute-mapper
        consentRequired: false
        config:
          claim.name: department
          user.attribute: department
          jsonType.label: String
          id.token.claim: "true"
          access.token.claim: "true"
          userinfo.token.claim: "true"
```

## Definition Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Scope name (required) |
| `description` | string | Description |
| `protocol` | string | Protocol (openid-connect, saml) |
| `protocolMappers` | array | Protocol mapper configurations |
| `attributes` | map | Additional attributes |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kccs` | `keycloakclientscopes` |

```bash
kubectl get kccs
```
