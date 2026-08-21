# About

The Keycloak Operator is a Kubernetes operator developed by [**Hostzero**](https://hostzero.com) that manages Keycloak instances through the [Keycloak Admin API][1]. The overall goal is to provide a cloud-native management interface for Keycloak instances.

## Features

- **Declarative Configuration**: Manage Keycloak resources as Kubernetes Custom Resources
- **Automatic Synchronization**: Changes to CRs are automatically applied to Keycloak
- **Secret Management**: Client secrets are synced to Kubernetes Secrets; `configSecretRef` keeps IdP, LDAP, and other config values out of the CR ([Secret references](./crds/secrets.md))
- **Status Tracking**: Resource status reflects the current state in Keycloak
- **Finalizers**: Proper cleanup when resources are deleted

## Goals

* Manage Keycloak instances solely through Kubernetes resources
* Provide a GitOps-friendly way to manage Keycloak configuration
* Enable infrastructure-as-code for identity management
* Support multiple Keycloak instances from a single operator

## Non-Goals

* Manage the deployment of Keycloak instances (use Keycloak Operator or Helm for that)
* Support other IdM solutions than Keycloak

## Supported Resources

The operator manages Keycloak through a set of Custom Resource Definitions covering instances, realms, clients, users, groups, roles, identity providers, federation components, authentication flows, organizations, and more.

A minimal example looks like this:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    enabled: true
    displayName: My Realm
```

See [Custom Resource Definitions](./crds.md) for the full list of supported resources and their schemas.

## Enterprise Support

<p align="center">
  <a href="https://hostzero.com">
    <img src="./assets/hostzero-logo.svg" alt="Hostzero" width="180">
  </a>
</p>

This operator is developed and maintained by [**Hostzero GmbH**](https://hostzero.com), a provider of sovereign IT infrastructure and security solutions based in Germany.

**For organizations with critical infrastructure needs (KRITIS), we offer:**

| Service | Description |
|---------|-------------|
| Enterprise Support | SLA-backed support with guaranteed response times |
| Security Consulting | Hardening, compliance audits, and KRITIS certification support |
| On-Premises Deployment | Air-gapped and sovereign cloud deployments |
| Incident Response | 24/7 emergency support for production environments |
| Training | Workshops and certification programs |

→ [Contact Hostzero](https://hostzero.com/contact-us) for enterprise solutions

## License

This project is licensed under the MIT License. See the [LICENSE](https://github.com/Hostzero-GmbH/keycloak-operator/blob/main/LICENSE) file for details.

[1]: https://www.keycloak.org/docs-api/latest/rest-api/
