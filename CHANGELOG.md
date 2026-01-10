# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.1.0] - 2024-XX-XX

### Added

- Initial release
- KeycloakInstance and ClusterKeycloakInstance resources for connecting to Keycloak servers
- KeycloakRealm and ClusterKeycloakRealm resources for managing realms
- KeycloakClient resource with automatic client secret synchronization
- KeycloakClientScope resource for managing client scopes
- KeycloakProtocolMapper resource for token claim configuration
- KeycloakUser resource for user management
- KeycloakUserCredential resource for password management
- KeycloakGroup resource for group management
- KeycloakRole resource for realm and client roles
- KeycloakRoleMapping resource for role assignments
- KeycloakIdentityProvider resource for external identity providers
- KeycloakComponent resource for LDAP federation and key providers
- KeycloakOrganization resource for Keycloak 26+ organizations
- Helm chart for easy deployment
- Prometheus metrics for monitoring
- Leader election for high availability
- Comprehensive E2E test suite

[Unreleased]: https://github.com/Hostzero-GmbH/keycloak-operator/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Hostzero-GmbH/keycloak-operator/releases/tag/v0.1.0
