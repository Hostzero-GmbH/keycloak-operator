# ClusterKeycloakInstance

`ClusterKeycloakInstance` is the cluster-scoped counterpart of
[`KeycloakInstance`](keycloakinstance.md). Resources in any namespace can
reference it, making it useful for a shared Keycloak server on a multi-tenant
platform.

## Example

### Password grant

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: central-keycloak
spec:
  baseUrl: https://keycloak.example.com
  auth:
    passwordGrant:
      secretRef:
        name: keycloak-admin
        namespace: keycloak-system
```

### Service-account client

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: ClusterKeycloakInstance
metadata:
  name: central-keycloak
spec:
  baseUrl: https://keycloak.example.com
  auth:
    clientCredentials:
      secretRef:
        name: keycloak-operator-client
        namespace: keycloak-system
```

## Authentication

Same rules as [`KeycloakInstance`](keycloakinstance.md): exactly one of
`auth.passwordGrant` / `auth.clientCredentials`; passwords and client secrets
always live in a Secret; `username` / `clientId` may be inlined.

The only difference: `secretRef.namespace` is **required** because the resource
is cluster-scoped.

## TLS

`spec.tls` mirrors the namespaced `KeycloakInstance.spec.tls` shape, with one
difference: every `namespace` field is **required**.

```yaml
spec:
  tls:
    caCert:
      configMapRef:
        name: keycloak-ca
        namespace: keycloak-system
        # key defaults to "ca.crt"
    # insecureSkipVerify: true  # disables verification, ignores caCert
```

Exactly one of `caCert.secretRef` / `caCert.configMapRef` may be set; setting
both is rejected by admission.

## Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `baseUrl` | string | URL of the Keycloak server | Yes |
| `auth.passwordGrant` / `auth.clientCredentials` | object | Authentication method (exactly one) | Yes |
| `auth.passwordGrant.username` | string | Inline admin username (overrides `secretRef.usernameKey`) | No |
| `auth.passwordGrant.secretRef.name` | string | Name of the credentials Secret | Yes |
| `auth.passwordGrant.secretRef.namespace` | string | Namespace of the credentials Secret | Yes |
| `auth.passwordGrant.secretRef.usernameKey` | string | Secret key for the username | No (default `username`) |
| `auth.passwordGrant.secretRef.passwordKey` | string | Secret key for the password | No (default `password`) |
| `auth.clientCredentials.clientId` | string | Inline client id (overrides `secretRef.clientIdKey`) | No |
| `auth.clientCredentials.secretRef.name` | string | Name of the client-credentials Secret | Yes |
| `auth.clientCredentials.secretRef.namespace` | string | Namespace of the client-credentials Secret | Yes |
| `auth.clientCredentials.secretRef.clientIdKey` | string | Secret key for the client id | No (default `client-id`) |
| `auth.clientCredentials.secretRef.clientSecretKey` | string | Secret key for the client secret | No (default `client-secret`) |
| `realm` | string | Admin realm name | No (default `master`) |
| `tls.caCert.secretRef` / `tls.caCert.configMapRef` | object | PEM-encoded CA bundle source (exactly one) | No |
| `tls.insecureSkipVerify` | bool | Disable TLS verification (overrides `caCert`) | No (default `false`) |
| `token.*` | object | Token cache configuration | No |

## Comparison with KeycloakInstance

| Aspect | KeycloakInstance | ClusterKeycloakInstance |
|--------|------------------|-------------------------|
| Scope | Namespaced | Cluster |
| Secret namespace | Optional (defaults to same as resource) | Required |
| Accessible from | Same namespace only | Any namespace |
| Short name | `kci` | `ckci` |

## Migrating from the pre-`auth` shape

The legacy `spec.credentials` / `spec.client` blocks have been replaced by
`spec.auth`. Migrate existing manifests as shown in the
[KeycloakInstance migration guide](keycloakinstance.md#migrating-from-the-pre-auth-shape);
the only difference is that `secretRef.namespace` must always be set for
cluster-scoped resources.
