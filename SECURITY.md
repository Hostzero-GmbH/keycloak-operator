# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

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

## Security Best Practices

When deploying the operator:

1. **Use RBAC**: Deploy with minimal required permissions
2. **Network Policies**: Restrict operator network access to only Keycloak
3. **Secrets Management**: Use Kubernetes secrets or external secret managers
4. **Image Verification**: Verify container image signatures when available
5. **Keep Updated**: Run the latest stable version
