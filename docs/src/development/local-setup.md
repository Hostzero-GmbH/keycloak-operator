# Local Setup

Set up a local development environment.

## Prerequisites

Install required tools:

```bash
# Go
brew install go

# Docker
brew install --cask docker

# Kind
brew install kind

# Kubectl
brew install kubectl

# Helm
brew install helm
```

## Clone and Build

```bash
git clone https://github.com/hostzero/keycloak-operator.git
cd keycloak-operator

# Download dependencies
go mod download

# Generate CRDs and code
make generate
make manifests

# Build binary
make build
```

## Create Kind Cluster

```bash
make kind-create
```

This creates a Kind cluster with Keycloak deployed.

## Run Locally

Run the operator against the Kind cluster:

```bash
# Install CRDs
make install

# Run operator locally
make run
```

## Deploy to Kind

Build and deploy to Kind:

```bash
# Build image
make docker-build IMG=keycloak-operator:dev

# Load to Kind
kind load docker-image keycloak-operator:dev --name keycloak-operator-e2e

# Deploy
make helm-install-dev
```

## Iterate

1. Make code changes
2. Run `make generate manifests` if CRDs changed
3. Run `make build` to verify compilation
4. Run `make test` for unit tests
5. Deploy to Kind for integration testing

## IDE Setup

### VS Code

Recommended extensions:
- Go
- YAML
- Kubernetes

settings.json:
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"]
}
```

### GoLand

Enable:
- Go modules integration
- File watchers for code generation
