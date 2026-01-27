# Exporting Keycloak Resources

The Keycloak Operator includes an `export` command that extracts resources from an existing Keycloak instance and generates Kubernetes CRD manifests. This is useful for:

- **Migration**: Moving from manual Keycloak configuration to operator-managed resources
- **Discovery**: Generating manifests from an existing Keycloak setup
- **Backup**: Creating declarative representations of your Keycloak configuration

## Quick Start

Export a realm to stdout:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm
```

## Running the Export Command

The export command is included in the operator container image. Run it via Docker:

### Direct Connection Mode

Connect directly to a Keycloak instance using URL and credentials:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --target-namespace production \
  --instance-ref keycloak-prod
```

### From Existing KeycloakInstance CR

If you already have the operator deployed with a `KeycloakInstance` configured, you can reuse those connection details:

```bash
docker run --rm -v ~/.kube:/root/.kube ghcr.io/hostzero-gmbh/keycloak-operator export \
  --from-instance my-keycloak \
  --namespace keycloak-operator \
  --realm my-realm
```

This reads the connection URL and credentials from the existing `KeycloakInstance` CR and its associated Secret.

For cluster-scoped instances:

```bash
docker run --rm -v ~/.kube:/root/.kube ghcr.io/hostzero-gmbh/keycloak-operator export \
  --from-cluster-instance my-cluster-keycloak \
  --realm my-realm
```

## Output Options

### Stdout (Default)

Output all manifests as multi-document YAML, suitable for piping:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  > manifests.yaml
```

Or apply directly:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  | kubectl apply -f -
```

### Single File

Write all manifests to a single file:

```bash
docker run --rm -v $(pwd):/output ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --output /output/manifests.yaml
```

### Directory Structure

Create an organized directory hierarchy:

```bash
docker run --rm -v $(pwd)/manifests:/output ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --output-dir /output
```

This creates:

```
manifests/
  realm.yaml
  clients/
    my-app.yaml
    another-client.yaml
  users/
    john-doe.yaml
  groups/
    admin-group.yaml
  roles/
    custom-role.yaml
  ...
```

## Filtering Resources

### Include Specific Types

Export only certain resource types:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --include clients,users,groups
```

### Exclude Types

Skip certain resource types:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --exclude role-mappings,protocol-mappers
```

### Resource Types

Available resource types for filtering:

| Type | Description |
|------|-------------|
| `realm` | The realm itself |
| `clients` | OAuth2/OIDC clients |
| `client-scopes` | Client scopes |
| `users` | User accounts |
| `groups` | User groups |
| `roles` | Realm and client roles |
| `role-mappings` | Role assignments to users/groups |
| `identity-providers` | External identity providers (SAML, OIDC, etc.) |
| `components` | LDAP federation, key providers, etc. |
| `protocol-mappers` | Token claim mappers |
| `organizations` | Organizations (Keycloak 26+) |

### Skip Built-in Resources

By default, Keycloak's built-in resources are skipped (`--skip-defaults=true`). These include:

- Default clients: `account`, `account-console`, `admin-cli`, `broker`, `realm-management`, `security-admin-console`
- Default client scopes: `address`, `email`, `offline_access`, `phone`, `profile`, `roles`, `web-origins`, etc.
- Default roles: `offline_access`, `uma_authorization`, `default-roles-{realm}`
- Service account users (prefixed with `service-account-`)

To include built-in resources:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --skip-defaults=false
```

## Manifest Generation Options

### Target Namespace

Set the namespace for generated manifests:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --target-namespace production
```

### Instance Reference

Set the KeycloakInstance reference for all generated resources:

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --instance-ref keycloak-prod
```

### Realm Reference

Override the realm reference name (defaults to the sanitized realm name):

```bash
docker run --rm ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm \
  --realm-ref production-realm
```

## Security Considerations

### Secrets Are Never Exported

The export command **never exports secrets**. This includes:

- Client secrets
- User passwords
- Identity provider client secrets
- LDAP bind credentials

You must create these secrets separately and configure the CRs to reference them.

### Password Handling

Pass passwords via environment variable instead of command line:

```bash
export KEYCLOAK_PASSWORD="your-password"
docker run --rm -e KEYCLOAK_PASSWORD ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm my-realm
```

## Example Workflow: Migrating to the Operator

1. **Export existing configuration:**

```bash
docker run --rm -v $(pwd)/manifests:/output ghcr.io/hostzero-gmbh/keycloak-operator export \
  --url https://keycloak.example.com \
  --username admin \
  --password "$KEYCLOAK_PASSWORD" \
  --realm production \
  --target-namespace keycloak \
  --instance-ref keycloak-prod \
  --output-dir /output
```

2. **Review and customize manifests:**

```bash
# Review generated files
ls -la manifests/

# Edit as needed (add client secrets, customize settings)
vim manifests/clients/my-app.yaml
```

3. **Create required secrets:**

```bash
# Create client secrets referenced by the manifests
kubectl create secret generic my-app-credentials \
  --namespace keycloak \
  --from-literal=client-secret=your-client-secret
```

4. **Deploy the operator (if not already installed):**

```bash
helm install keycloak-operator oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

5. **Create the KeycloakInstance:**

```bash
# Create credentials secret
kubectl create secret generic keycloak-admin \
  --namespace keycloak \
  --from-literal=username=admin \
  --from-literal=password=your-admin-password

# Apply instance configuration
kubectl apply -f - <<EOF
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: keycloak-prod
  namespace: keycloak
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin
EOF
```

6. **Apply the exported manifests:**

```bash
kubectl apply -f manifests/
```

## Command Reference

```
Usage: keycloak-operator export [options]

Connection Options (choose one mode):
  --url           Keycloak server URL
  --username      Admin username
  --password      Admin password (or use KEYCLOAK_PASSWORD env var)

  --from-instance          Name of KeycloakInstance CR
  --from-cluster-instance  Name of ClusterKeycloakInstance CR
  --namespace              Namespace of KeycloakInstance

Export Options:
  --realm         Realm to export (required)

Output Options:
  --output        Output file path (default: stdout)
  --output-dir    Output directory (creates file structure)

Manifest Options:
  --target-namespace  Namespace for generated manifests (default: "default")
  --instance-ref      KeycloakInstance name to reference
  --realm-ref         KeycloakRealm name to reference

Filtering Options:
  --include       Resource types to include (comma-separated)
  --exclude       Resource types to exclude (comma-separated)
  --skip-defaults Skip built-in Keycloak resources (default: true)

General Options:
  --verbose       Enable verbose output
```
