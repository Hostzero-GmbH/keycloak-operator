# Installation

There are several ways to install the Keycloak Operator:

## Helm Chart (Recommended)

The preferred way to install the Keycloak Operator is using the provided Helm chart.

```shell
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

For detailed Helm configuration options, see the [Helm Chart documentation](./installation/helm.md).

## Kustomize

You can also deploy using kustomize:

```shell
# Install CRDs
kubectl apply -k config/crd

# Deploy the operator
kubectl apply -k config/default
```

## From Source

For development or customization:

```shell
# Clone the repository
git clone https://github.com/hostzero/keycloak-operator.git
cd keycloak-operator

# Install CRDs
make install

# Run the operator locally
make run
```

## Next Steps

After installation, proceed to the [Quick Start](./installation/quick-start.md) guide to create your first Keycloak resources.
