# Contributing to Keycloak Operator

Thank you for your interest in contributing to the Keycloak Operator! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Bugs

Before creating a bug report:

1. Check the [existing issues](https://github.com/Hostzero-GmbH/keycloak-operator/issues) to avoid duplicates
2. Collect relevant information:
   - Kubernetes version (`kubectl version`)
   - Keycloak version
   - Operator version (check the pod image tag)
   - Relevant logs from the operator pod
   - CRD manifests that reproduce the issue

When creating a bug report, please use the bug report template and include as much detail as possible.

### Suggesting Features

Feature suggestions are welcome! Please:

1. Check if the feature has already been requested
2. Describe the use case clearly
3. Explain how the feature would benefit users

### Pull Requests

1. **Fork and clone** the repository
2. **Create a branch** for your changes: `git checkout -b feature/my-feature`
3. **Make your changes** following our coding standards
4. **Test your changes** locally using the Kind cluster
5. **Update documentation** if needed
6. **Submit a pull request** with a clear description

## Development Setup

### Prerequisites

- Go 1.22+
- Docker
- kubectl
- Kind (`brew install kind` or `go install sigs.k8s.io/kind@latest`)
- Helm

### Quick Start

```bash
# Clone the repository
git clone https://github.com/Hostzero-GmbH/keycloak-operator.git
cd keycloak-operator

# Create a Kind cluster with Keycloak and deploy the operator
make kind-all

# Check operator logs
make kind-logs

# Apply sample resources
kubectl apply -f config/samples/

# Run tests
make test

# Run E2E tests
make kind-test
```

### Building

```bash
# Build the operator binary
make build

# Build Docker image
make docker-build

# Generate CRDs and RBAC manifests
make manifests

# Generate Go code (deepcopy, etc.)
make generate
```

### Code Style

- Follow standard Go idioms and best practices
- Run `make fmt` before committing
- Ensure `make lint` passes without errors
- Write tests for new functionality
- Keep functions focused and well-documented

### Commit Messages

We follow conventional commit messages:

- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `chore:` for maintenance tasks
- `refactor:` for code refactoring
- `test:` for adding tests

Example: `feat: add support for KeycloakOrganization resources`

### Testing

```bash
# Run unit tests
make test

# Run linter
make lint

# Run E2E tests (requires Kind cluster)
make kind-test
```

## Project Structure

```
keycloak-operator/
├── api/v1beta1/          # API types (CRDs)
├── cmd/                  # Operator entrypoint
├── internal/
│   ├── controller/       # Reconciliation logic
│   └── keycloak/         # Keycloak client wrapper
├── config/
│   ├── crd/              # CRD manifests
│   ├── manager/          # Operator deployment
│   ├── rbac/             # RBAC configuration
│   └── samples/          # Example resources
├── test/e2e/             # End-to-end tests
├── charts/               # Helm chart
├── docs/                 # Documentation (mdBook)
└── hack/                 # Development scripts
```

## Releasing

Releases are automated via GitHub Actions. To create a release:

1. Update version in `charts/keycloak-operator/Chart.yaml` (both `version` and `appVersion`)

2. Update `CHANGELOG.md` with release notes

3. Create and push a tag:
   ```bash
   git tag v0.2.0
   git push origin v0.2.0
   ```

4. The CI will automatically:
   - Run tests
   - Build and push Docker images to `ghcr.io/hostzero-gmbh/keycloak-operator`
   - Publish the Helm chart to `oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator`
   - Create a GitHub Release with CRDs and Helm chart archive

## Getting Help

- Open a [discussion](https://github.com/Hostzero-GmbH/keycloak-operator/discussions) for questions
- Check the [documentation](https://keycloak-operator.hostzero.com)
- Review existing [issues](https://github.com/Hostzero-GmbH/keycloak-operator/issues)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
