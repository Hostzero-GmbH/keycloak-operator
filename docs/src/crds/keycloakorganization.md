# KeycloakOrganization

Manages organizations (Keycloak 26+).

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `realmRef` | ResourceRef | Yes | Reference to KeycloakRealm |
| `definition` | object | Yes | Organization representation |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | bool | Organization is synced |
| `status` | string | Human-readable status |
| `message` | string | Detailed message |
| `organizationId` | string | Keycloak organization UUID |

## Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: acme-corp
spec:
  realmRef:
    name: my-app
  definition:
    name: acme-corp
    enabled: true
    domains:
      - name: acme.com
        verified: true
    attributes:
      industry:
        - technology
```

## Requirements

- Keycloak 26.0 or later
- Organizations feature enabled in Keycloak

## Features

Organizations in Keycloak 26+ provide:

- Multi-tenancy support
- Domain-based user assignment
- Organization-specific identity providers
- Centralized user management per organization
