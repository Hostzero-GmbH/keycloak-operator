# Security Policy

## Supported Versions

Only the latest minor release receives security fixes. Upgrade to the latest
version before reporting.

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please report it responsibly.

### How to Report

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please report security vulnerabilities by emailing:

**security@hostzero.com**

Include the following information:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes (if available)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt within 48 hours
- **Assessment**: We will assess the vulnerability and determine its severity
- **Updates**: We will keep you informed of our progress
- **Resolution**: We aim to resolve critical vulnerabilities within 30 days
- **Disclosure**: We will coordinate disclosure timing with you

### Scope

This security policy covers:

- The Keycloak Operator codebase
- The Helm chart
- Official container images published to ghcr.io

### Out of Scope

- Keycloak itself (report to the Keycloak project)
- Third-party dependencies (report to the respective maintainers, but let us know)
- Infrastructure not managed by us

## Security Model

The operator reads Secrets with its own service account and sends their
contents to the Keycloak server named in the custom resource. The right to
create or update a resource is therefore equivalent to reading every Secret
that resource can reference:

| Permission | Equivalent to |
|------------|---------------|
| `create`/`update` `KeycloakInstance` (and other namespaced CRs) in namespace X | Read any Secret in namespace X |
| `create`/`update` `ClusterKeycloakInstance` or `ClusterKeycloakRealm` | Read any Secret in the cluster |

Namespaced resources only ever read Secrets and ConfigMaps from their own
namespace. Do not grant the cluster-scoped kinds to anyone who is not already
a cluster administrator, and do not grant the namespaced kinds to users who
should not be able to read Secrets in that namespace.

If you need a stronger guarantee, add an admission policy (ValidatingAdmissionPolicy,
Kyverno, Gatekeeper) that restricts `spec.baseUrl` to your Keycloak hostnames.

## Security Best Practices

When deploying the operator:

1. **Use RBAC**: Deploy with minimal required permissions, see above
2. **Network Policies**: Restrict operator egress to the Keycloak server. The
   Helm chart ships a `NetworkPolicy` template for this
3. **Secrets Management**: Use Kubernetes secrets or external secret managers
4. **Image Verification**: Verify container image signatures when available
5. **Keep Updated**: Run the latest stable version
