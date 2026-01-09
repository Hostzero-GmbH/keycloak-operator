# Development

This section covers how to set up a development environment and contribute to the Keycloak Operator.

## Prerequisites

- Go 1.22+
- Docker
- kubectl
- Kind or Minikube
- Make

## Getting Started

1. Clone the repository:
   ```bash
   git clone https://github.com/hostzero/keycloak-operator.git
   cd keycloak-operator
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up the development environment:
   ```bash
   make kind-all
   ```

## Project Structure

```
keycloak-operator/
├── api/v1beta1/           # CRD type definitions
├── cmd/main.go            # Entry point
├── internal/
│   ├── controller/        # Reconciliation logic
│   └── keycloak/          # Keycloak client wrapper
├── config/
│   ├── crd/               # CRD manifests
│   ├── manager/           # Operator deployment
│   ├── rbac/              # RBAC configuration
│   └── samples/           # Example CRs
├── charts/                # Helm chart
├── hack/                  # Development scripts
├── test/
│   └── e2e/               # End-to-end tests
└── docs/                  # Documentation (mdBook)
```

## Development Workflow

See the specific guides:
- [Local Setup](./development/local-setup.md)
- [Testing](./development/testing.md)
- [Contributing](./development/contributing.md)
