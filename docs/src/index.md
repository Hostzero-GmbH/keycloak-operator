# About

The Keycloak Operator is a Kubernetes operator developed by [**Hostzero**](https://hostzero.com) that manages Keycloak instances through the [Keycloak Admin API][1]. The overall goal is to provide a cloud-native management interface for Keycloak instances.

## Features

- **Declarative Configuration**: Manage Keycloak resources as Kubernetes Custom Resources
- **Automatic Synchronization**: Changes to CRs are automatically applied to Keycloak
- **Secret Management**: Client secrets are automatically synced to Kubernetes Secrets
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

| Resource | Description |
|----------|-------------|
| `KeycloakInstance` | Connection to a Keycloak server (namespaced) |
| `ClusterKeycloakInstance` | Connection to a Keycloak server (cluster-scoped) |
| `KeycloakRealm` | Realm configuration (namespaced) |
| `ClusterKeycloakRealm` | Realm configuration (cluster-scoped) |
| `KeycloakClient` | OAuth2/OIDC client configuration |
| `KeycloakClientScope` | Client scope configuration |
| `KeycloakProtocolMapper` | Token claim mappers for clients/scopes |
| `KeycloakUser` | User management |
| `KeycloakUserCredential` | User password management |
| `KeycloakGroup` | Group management |
| `KeycloakRole` | Realm and client role definitions |
| `KeycloakRoleMapping` | Role-to-user/group assignments |
| `KeycloakIdentityProvider` | External identity provider configuration |
| `KeycloakComponent` | LDAP federation, key providers, etc. |
| `KeycloakOrganization` | Organization management (Keycloak 26+) |

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

→ [Contact Hostzero](https://hostzero.com/contact) for enterprise solutions

## License

This project is licensed under the MIT License. See the [LICENSE](https://github.com/Hostzero-GmbH/keycloak-operator/blob/main/LICENSE) file for details.

[1]: https://www.keycloak.org/docs-api/latest/rest-api/
