# Development Guide

Guide for developing and contributing to the Keycloak Operator.

## Prerequisites

- Go 1.21+
- Docker
- kubectl
- Kind
- Helm 3

## Getting Started

```bash
# Clone the repository
git clone https://github.com/hostzero/keycloak-operator.git
cd keycloak-operator

# Install dependencies
go mod download

# Generate code
make generate

# Build
make build
```

## Project Structure

```
.
├── api/v1beta1/          # CRD type definitions
├── cmd/                  # Main entry point
├── config/               # Kustomize manifests
├── charts/               # Helm chart
├── docs/                 # Documentation (mdBook)
├── hack/                 # Development scripts
├── internal/
│   ├── controller/       # Reconcilers
│   └── keycloak/         # Keycloak API client
└── test/
    └── e2e/              # End-to-end tests
```

## Development Sections

- [Local Setup](./development/local-setup.md) - Set up local development environment
- [Testing](./development/testing.md) - Run tests
- [Contributing](./development/contributing.md) - Contribution guidelines
