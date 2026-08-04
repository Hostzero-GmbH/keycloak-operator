# KeycloakClientScope

> **Identifier field:** Set the client scope name in the `spec.name` field. It is required and immutable once set. A `name` inside `spec.definition` is tolerated only when it matches `spec.name`; a conflicting value is rejected.

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
  
  # Option 2: Reference to a ClusterKeycloakRealm
  clusterRealmRef:
    name: my-cluster-realm
  
  # Required: Client scope definition
  name: my-scope
  definition:
    protocol: openid-connect
    # ... any other properties
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  message: "Client scope synchronized successfully"
  resourcePath: "/admin/realms/my-realm/client-scopes/12345678-..."
  instance:
    instanceRef: my-keycloak
  realm:
    realmRef: my-realm
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
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
  name: profile-extended
  definition:
    description: Extended profile information
    protocol: openid-connect
```

### Scope with Protocol Mappers

Protocol mappers are declared as separate [KeycloakProtocolMapper](keycloakprotocolmapper.md)
resources. Keycloak ignores `protocolMappers` sent on the client scope update
endpoint, so the operator rejects the key inside `definition` rather than
accepting edits it cannot apply.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClientScope
metadata:
  name: department-scope
spec:
  realmRef:
    name: my-realm
  name: department
  definition:
    description: Department information
    protocol: openid-connect
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakProtocolMapper
metadata:
  name: department
spec:
  clientScopeRef:
    name: department-scope
  name: department
  definition:
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
| `attributes` | map | Additional attributes |

`protocolMappers` is rejected here; use [KeycloakProtocolMapper](keycloakprotocolmapper.md).

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kccs` | `keycloakclientscopes` |

```bash
kubectl get kccs
```
