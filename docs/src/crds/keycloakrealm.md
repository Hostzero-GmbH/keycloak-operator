# KeycloakRealm

A `KeycloakRealm` represents a realm within a Keycloak instance.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  # One of instanceRef or clusterInstanceRef must be specified
  
  # Option 1: Reference to a namespaced KeycloakInstance
  instanceRef:
    name: my-keycloak
    namespace: default  # Optional
  
  # Option 2: Reference to a ClusterKeycloakInstance
  # clusterInstanceRef:
  #   name: my-cluster-instance
  
  # Optional: Realm name in Keycloak (defaults to metadata.name)
  realmName: my-realm
  
  # Required: Realm definition (Keycloak RealmRepresentation)
  definition:
    realm: my-realm
    displayName: My Realm
    enabled: true
    # ... any other Keycloak realm properties
```

## Status

```yaml
status:
  ready: true
  status: "Ready"
  message: "Realm synchronized successfully"
  resourcePath: "/admin/realms/my-realm"
  instance:
    instanceRef: my-keycloak
  conditions:
    - type: Ready
      status: "True"
      reason: Synchronized
```

## Example

### Basic Realm

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app-realm
spec:
  instanceRef:
    name: production-keycloak
  definition:
    realm: my-app
    displayName: My Application
    enabled: true
```

### With ClusterKeycloakInstance

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app-realm
spec:
  clusterInstanceRef:
    name: central-keycloak
  definition:
    realm: my-app
    displayName: My Application
    enabled: true
```

### Full Configuration

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: production-realm
spec:
  instanceRef:
    name: production-keycloak
  definition:
    realm: production
    displayName: Production Realm
    enabled: true
    
    # Login settings
    registrationAllowed: false
    registrationEmailAsUsername: true
    loginWithEmailAllowed: true
    duplicateEmailsAllowed: false
    resetPasswordAllowed: true
    rememberMe: true
    
    # Session settings
    ssoSessionIdleTimeout: 1800
    ssoSessionMaxLifespan: 36000
    accessTokenLifespan: 300
    
    # Security settings
    bruteForceProtected: true
    permanentLockout: false
    maxFailureWaitSeconds: 900
    minimumQuickLoginWaitSeconds: 60
    waitIncrementSeconds: 60
    quickLoginCheckMilliSeconds: 1000
    maxDeltaTimeSeconds: 43200
    failureFactor: 5
    
    # Themes
    loginTheme: keycloak
    accountTheme: keycloak
    adminTheme: keycloak
    emailTheme: keycloak
    
    # SMTP settings
    smtpServer:
      host: smtp.example.com
      port: "587"
      fromDisplayName: My App
      from: noreply@example.com
      starttls: "true"
      auth: "true"
      user: smtp-user
      password: smtp-password
```

## Definition Properties

The `definition` field accepts any property from the [Keycloak RealmRepresentation](https://www.keycloak.org/docs-api/latest/rest-api/index.html#RealmRepresentation).

Common properties:

| Property | Type | Description |
|----------|------|-------------|
| `realm` | string | Realm name (required) |
| `displayName` | string | Display name for the realm |
| `enabled` | boolean | Whether the realm is enabled |
| `registrationAllowed` | boolean | Allow user registration |
| `loginWithEmailAllowed` | boolean | Allow login with email |
| `ssoSessionIdleTimeout` | integer | SSO session idle timeout (seconds) |
| `accessTokenLifespan` | integer | Access token lifespan (seconds) |

## Preserving Realm on Deletion

To keep the realm in Keycloak when deleting the CR:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
  annotations:
    keycloak.hostzero.com/preserve-resource: "true"
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
```

See [Common Patterns](../crds.md#preserving-resources-on-deletion) for more details.

## Short Names

| Alias | Full Name |
|-------|-----------|
| `kcrm` | `keycloakrealms` |

```bash
kubectl get kcrm
```
