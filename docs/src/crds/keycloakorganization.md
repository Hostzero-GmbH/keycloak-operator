# KeycloakOrganization

> **Identifier field:** Set the organization name in the `spec.name` field. It is required and immutable once set. A `name` inside `spec.definition` is tolerated only when it matches `spec.name`; a conflicting value is rejected.

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
  
  # Required: Organization definition (Keycloak OrganizationRepresentation)
  name: ACME Corporation
  definition:
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
  status: "Ready"
  organizationID: "12345678-1234-1234-1234-123456789abc"
  message: "Organization synchronized successfully"
  resourcePath: "/admin/realms/my-realm/organizations/12345678-..."
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

### Basic Organization

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakOrganization
metadata:
  name: my-org
spec:
  realmRef:
    name: my-realm
  name: My Organization
  definition:
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
  name: Example Organization
  definition:
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
  name: Enterprise Organization
  definition:
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
  name: Global Organization
  definition:
    enabled: true
```

## Definition Properties

Common properties from [Keycloak OrganizationRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#OrganizationRepresentation):

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Organization name (required) |
| `alias` | string | URL-friendly identifier |
| `description` | string | Description of the organization |
| `redirectUrl` | string | URL users are sent to after they have joined the organization |
| `enabled` | boolean | Whether organization is enabled |
| `domains` | array | Associated email domains |
| `attributes` | map | Custom organization attributes |

The definition is sent to Keycloak as it is, with `name` set from `spec.name`, so every field of the Keycloak `OrganizationRepresentation` is supported. A key Keycloak does not know is rejected by Keycloak (`400 Unrecognized field`) and shows up in the `Ready` condition message as a failed create or update.

### Keys Keycloak Ignores

Keycloak accepts `id`, `identityProviders`, `members` and `groups` in the organization representation and discards them: the `id` is generated, and the other three are managed through their own endpoints. They are neither sent nor compared, so they cannot keep the organization out of sync, and the `Ready` condition message names them:

```
Organization synchronized; ignored definition keys: id, identityProviders
```

Link identity providers with `organizationRef` on the [KeycloakIdentityProvider](./keycloakidentityprovider.md) instead.

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

## Reconciliation

Reconciliation is idempotent. On each periodic resync the operator fetches the current organization from Keycloak and compares it against the spec; the update PUT is skipped when they already match, so an unchanged organization does not generate redundant writes or log noise. `domains[].verified` is set by Keycloak on read and is ignored in the comparison, so leaving it unset (or out of sync with the server) does not trigger a perpetual update loop.

Only the keys the resource declares are compared. Keycloak's organization update replaces `description`, `redirectUrl` and `domains` with whatever the request carries, so when an update is needed the declared keys are applied on top of the organization Keycloak currently returns. A field the resource does not declare, such as a `redirectUrl` set in the Admin Console, is left as it is. A declared list or map (`domains`, `attributes`) replaces the one in Keycloak as a whole.

Because a field the resource does not declare is left alone, removing a key from the definition does not clear it in Keycloak. To clear a field, declare it empty, for example `redirectUrl: ""`, `description: ""` or `domains: []`. Keycloak then returns no value for it, and that counts as in sync.

## Linking Identity Providers

To map an identity provider to this organization, set `organizationRef` on the [KeycloakIdentityProvider](./keycloakidentityprovider.md) to this CR's name. The operator links the identity provider through this organization's identity-provider endpoint once both are ready; do not copy `status.organizationID` into the identity provider definition by hand.

## Notes

- Organizations are immutable by ID - Keycloak generates the id on create and it cannot be changed, so an `id` in the definition is ignored
- The `alias` is used in URLs and should be URL-safe
- Verified domains can be used for automatic user association based on email
- Use attributes for custom metadata and configuration
