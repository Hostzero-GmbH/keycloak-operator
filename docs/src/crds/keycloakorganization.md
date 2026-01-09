# KeycloakOrganization

A `KeycloakOrganization` represents an organization within a Keycloak realm.

> **Note:** Organizations require **Keycloak 26.0.0 or later**. Attempting to use this resource with earlier Keycloak versions will result in an error.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: acme-corp
spec:
  # One of realmRef or clusterRealmRef must be specified
  realmRef:
    name: my-realm
    namespace: default  # Optional
  
  # Required: Organization definition (Keycloak OrganizationRepresentation)
  definition:
    name: ACME Corporation
    alias: acme
    description: ACME Corp organization
    enabled: true
    domains:
      - name: acme.com
        verified: true
    attributes:
      industry:
        - Technology
```

## Status

```yaml
status:
  ready: true
  organizationID: "12345678-1234-1234-1234-123456789abc"
  message: "Organization synchronized successfully"
```

## Examples

### Basic Organization

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: my-org
spec:
  realmRef:
    name: my-realm
  definition:
    name: My Organization
    enabled: true
```

### Organization with Domains

Organizations can be associated with email domains. Users with matching email domains can be automatically associated with the organization.

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: example-org
spec:
  realmRef:
    name: my-realm
  definition:
    name: Example Organization
    alias: example
    description: An example organization with verified domains
    enabled: true
    domains:
      - name: example.com
        verified: true
      - name: example.org
        verified: false
```

### Organization with Custom Attributes

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: enterprise-org
spec:
  realmRef:
    name: my-realm
  definition:
    name: Enterprise Organization
    alias: enterprise
    enabled: true
    attributes:
      tier:
        - enterprise
      maxUsers:
        - "1000"
      supportLevel:
        - premium
```

### Organization with Cluster-Scoped Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: global-org
spec:
  clusterRealmRef:
    name: shared-realm
  definition:
    name: Global Organization
    enabled: true
```

## Definition Properties

Common properties from [Keycloak OrganizationRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#OrganizationRepresentation):

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Organization name (required) |
| `alias` | string | URL-friendly identifier |
| `description` | string | Description of the organization |
| `enabled` | boolean | Whether organization is enabled |
| `domains` | array | Associated email domains |
| `attributes` | map | Custom organization attributes |

### Domain Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Domain name (e.g., "example.com") |
| `verified` | boolean | Whether the domain is verified |

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcorg` | `keycloakorganizations` |

```bash
kubectl get kcorg
```

## Requirements

- **Keycloak 26.0.0+**: Organizations are a feature introduced in Keycloak 26. The operator will report an error if you try to create an organization on an older Keycloak version.
- **Organizations must be enabled**: The organization feature must be enabled in the realm settings.

## Notes

- Organizations are immutable by ID - once created, the `id` field cannot be changed
- The `alias` is used in URLs and should be URL-safe
- Verified domains can be used for automatic user association based on email
- Use attributes for custom metadata and configuration
