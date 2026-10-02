# Secret references

Sensitive values belong in Kubernetes Secrets, not in Custom Resource YAML. The operator has two kinds of secret fields.

## `configSecretRef`

CRs whose Keycloak payload has a `definition.config` map can merge every key from a Secret into that map before the operator talks to Keycloak:

- [KeycloakIdentityProvider](./keycloakidentityprovider.md)
- [KeycloakComponent](./keycloakcomponent.md)
- [KeycloakProtocolMapper](./keycloakprotocolmapper.md)
- [KeycloakIdentityProviderMapper](./keycloakidentityprovidermapper.md)
- [KeycloakRequiredAction](./keycloakrequiredaction.md)

Secret keys must already be the Keycloak config names (`bindCredential`, `clientSecret`, not `password`). Extra keys in the Secret are also pushed to Keycloak. Secret values win over the same key in `definition.config`.

The Secret must live in the same namespace as the CR. When the Secret changes, the operator re-reconciles.

```bash
kubectl create secret generic ldap-credentials \
  --from-literal=bindCredential=s3cret
```

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakComponent
metadata:
  name: ldap-federation
spec:
  realmRef:
    name: my-realm
  name: corporate-ldap
  configSecretRef:
    name: ldap-credentials
  definition:
    providerId: ldap
    providerType: org.keycloak.storage.UserStorageProvider
    config:
      enabled: ["true"]
      vendor: ["ad"]
      bindDn: ["cn=admin,dc=example,dc=com"]
```

Component config is a list of strings per key. The operator wraps each Secret value as `["…"]` (`bindCredential: "s3cret"` in the Secret becomes `["s3cret"]` in Keycloak). Identity providers, protocol mappers, identity provider mappers, and required actions keep string values.

## Other secret APIs

Do not use `configSecretRef` for these. They have their own typed fields:

| Resource | Field | Purpose |
|----------|-------|---------|
| [KeycloakClient](./keycloakclient.md) | `clientSecretRef` | OAuth client id / secret |
| [KeycloakRealm](./keycloakrealm.md) / [ClusterKeycloakRealm](./clusterkeycloakrealm.md) | `smtpSecretRef` | SMTP username and password |
| [KeycloakUserCredential](./keycloakusercredential.md) | `userSecret` | User password |
| [KeycloakInstance](./keycloakinstance.md) / [ClusterKeycloakInstance](./clusterkeycloakinstance.md) | `auth.*.secretRef` | Operator credentials to Keycloak |
