# KeycloakInstance

A `KeycloakInstance` represents a connection to a Keycloak server. It is the root resource for managing Keycloak configuration in a namespace.

## Specification

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
spec:
  baseUrl: https://keycloak.example.com

  # Optional: admin realm to authenticate against (default: master)
  realm: master

  # Required: exactly one of auth.passwordGrant or auth.clientCredentials
  auth:
    # Password grant via an admin user (e.g. master-realm admin)
    passwordGrant:
      # Optional inline username; when set, overrides secretRef.usernameKey
      username: admin
      secretRef:
        name: keycloak-admin
        # Optional: namespace of the secret (defaults to resource namespace)
        namespace: keycloak-operator
        # Optional: keys inside the secret (defaults shown)
        usernameKey: username
        passwordKey: password

    # OR: client_credentials grant via a confidential client
    clientCredentials:
      # Optional inline client_id; when set, overrides secretRef.clientIdKey
      clientId: keycloak-operator
      secretRef:
        name: keycloak-operator-client
        namespace: keycloak-operator
        clientIdKey: client-id
        clientSecretKey: client-secret

  # Optional: TLS verification for the Keycloak HTTPS endpoint
  tls:
    # Reference a PEM-encoded CA bundle from a Secret or ConfigMap
    caCert:
      # Use either secretRef OR configMapRef (mutually exclusive)
      secretRef:
        name: keycloak-ca
        # Optional: defaults to the KeycloakInstance namespace
        namespace: keycloak-operator
        # Optional: key inside the secret (default: ca.crt)
        key: ca.crt
      # configMapRef:
      #   name: keycloak-ca
      #   key: ca.crt
    # Disable TLS verification entirely. Do not use in production.
    insecureSkipVerify: false

  # Optional: token caching configuration
  token:
    secretName: keycloak-token-cache
    tokenKey: token
    expiresKey: expires
```

## TLS

`spec.tls` is optional. When omitted, the operator uses the system CA pool to
verify the Keycloak server certificate.

- `tls.caCert` references a PEM-encoded CA bundle from either a `Secret` or a
  `ConfigMap`. Exactly one of `secretRef` / `configMapRef` may be set; setting
  both is rejected by admission. The default key is `ca.crt`.
- `tls.insecureSkipVerify: true` disables certificate verification. When set,
  `caCert` is ignored.

## Authentication

Exactly one of `auth.passwordGrant` or `auth.clientCredentials` must be set; the
admission webhook rejects specs that omit both or set both. `auth.passwordGrant`
issues a password-grant token (typical for the master-realm admin user);
`auth.clientCredentials` issues a `client_credentials` token against a
confidential client / service account.

Username and client_id are not secrets, so they can be either inlined on the
spec or read from a key of the referenced Secret. When the inline field is set,
the corresponding `*Key` field on `secretRef` is ignored. Passwords and client
secrets always come from the Secret.

### Credentials Secret (password grant)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-admin
type: Opaque
stringData:
  username: admin
  password: your-secure-password
```

### Client credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: keycloak-operator-client
type: Opaque
stringData:
  client-id: keycloak-operator
  client-secret: your-client-secret
```

A one-liner:

```bash
kubectl create secret generic keycloak-operator-client \
  --from-literal=client-id=keycloak-operator \
  --from-literal=client-secret=$(openssl rand -hex 32)
```

## Status

```yaml
status:
  ready: true
  version: "26.0.0"
  status: "Ready"
  message: "Connected to Keycloak"
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
      lastTransitionTime: "2024-01-01T12:00:00Z"
```

## Examples

### Basic instance with password grant

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: production-keycloak
  namespace: keycloak-operator
spec:
  baseUrl: https://auth.example.com
  auth:
    passwordGrant:
      secretRef:
        name: keycloak-admin
```

### Service-account client

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: production-keycloak
spec:
  baseUrl: https://auth.example.com
  auth:
    clientCredentials:
      secretRef:
        name: keycloak-operator-client
```

## Migrating from the pre-`auth` shape

Earlier `v1beta1` revisions used `spec.credentials` and `spec.client` at the
top level. The fields have been replaced by `spec.auth`. Existing manifests
must be migrated; the operator rejects the old shape.

### Password grant

Before:

```yaml
spec:
  baseUrl: https://auth.example.com
  credentials:
    secretRef:
      name: keycloak-admin
      usernameKey: username
      passwordKey: password
```

After:

```yaml
spec:
  baseUrl: https://auth.example.com
  auth:
    passwordGrant:
      secretRef:
        name: keycloak-admin
        usernameKey: username
        passwordKey: password
```

### Client credentials

Before (client_id and client_secret were inlined in the spec):

```yaml
spec:
  baseUrl: https://auth.example.com
  credentials:
    secretRef:
      name: dummy-unused-admin
  client:
    id: keycloak-operator
    secret: my-client-secret
```

After (move the credentials into a Secret, drop the dummy `credentials` block):

```yaml
spec:
  baseUrl: https://auth.example.com
  auth:
    clientCredentials:
      secretRef:
        name: keycloak-operator-client
```

with a companion Secret:

```bash
kubectl create secret generic keycloak-operator-client \
  --from-literal=client-id=keycloak-operator \
  --from-literal=client-secret=my-client-secret
```

## Short names

| Alias | Full name |
|-------|-----------|
| `kci` | `keycloakinstances` |

```bash
kubectl get kci
```
