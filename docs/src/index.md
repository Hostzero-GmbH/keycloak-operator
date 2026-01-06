# Keycloak Operator

A Kubernetes operator for managing Keycloak resources declaratively.

## Overview

The Keycloak Operator allows you to manage your Keycloak configuration as Kubernetes custom resources. This enables GitOps workflows, version control, and automated deployment of your identity and access management configuration.

## Features

- **Declarative Configuration**: Define Keycloak resources as Kubernetes CRDs
- **GitOps Ready**: Store your Keycloak configuration in Git
- **Full Lifecycle Management**: Create, update, and delete resources automatically
- **Multi-Instance Support**: Manage multiple Keycloak instances from a single operator
- **Cluster-Scoped Resources**: Share instances and realms across namespaces
- **Keycloak 26+ Support**: Includes organization management for Keycloak 26+

## Supported Resources

| Resource | Description |
|----------|-------------|
| KeycloakInstance | Connection to a Keycloak server |
| KeycloakRealm | Keycloak realm configuration |
| KeycloakClient | OAuth2/OIDC clients |
| KeycloakUser | User accounts |
| KeycloakRole | Realm and client roles |
| KeycloakGroup | User groups |
| KeycloakClientScope | Client scopes |
| KeycloakRoleMapping | Role assignments to users |
| KeycloakUserCredential | User passwords |
| KeycloakProtocolMapper | Token mappers |
| KeycloakIdentityProvider | External identity providers |
| KeycloakComponent | Keycloak components (keys, LDAP, etc.) |
| KeycloakOrganization | Organizations (Keycloak 26+) |

## Quick Example

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: main
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin
---
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-app
spec:
  instanceRef:
    name: main
  definition:
    realm: my-app
    enabled: true
```

## Getting Started

Head over to the [Quick Start](./installation/quick-start.md) guide to deploy the operator in minutes.
