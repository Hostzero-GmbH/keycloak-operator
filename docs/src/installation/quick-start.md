# Quick Start

This guide will walk you through setting up the Keycloak Operator and creating your first managed resources.

## Prerequisites

* A running Kubernetes cluster
* `kubectl` installed and configured
* `helm` installed (optional, for Helm installation)
* A Keycloak instance (or use the provided Kind setup)

## Step 1: Install the Operator

### Option A: Using Helm (Recommended)

```bash
helm install keycloak-operator oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### Option B: Using Helm from Source

```bash
helm install keycloak-operator ./charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

### Option C: Using Kind (for development)

```bash
# This creates a Kind cluster with Keycloak and deploys the operator
make kind-all
```

## Step 2: Create Admin Credentials Secret

Create a secret containing your Keycloak admin credentials:

```bash
kubectl create secret generic keycloak-admin-credentials \
  --namespace keycloak-operator \
  --from-literal=username=admin \
  --from-literal=password=your-admin-password
```

## Step 3: Create a KeycloakInstance

Create a `KeycloakInstance` resource to connect the operator to your Keycloak server:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: my-keycloak
  namespace: keycloak-operator
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin-credentials
```

Apply it:

```bash
kubectl apply -f keycloak-instance.yaml
```

Verify the connection:

```bash
kubectl get keycloakinstances -n keycloak-operator
```

You should see:

```
NAME          READY   URL                           VERSION   AGE
my-keycloak   true    https://keycloak.example.com  26.0.0    30s
```

## Step 4: Create a Realm

Create a realm in your Keycloak instance:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakRealm
metadata:
  name: my-realm
  namespace: keycloak-operator
spec:
  instanceRef:
    name: my-keycloak
  definition:
    realm: my-realm
    displayName: My Application Realm
    enabled: true
    registrationAllowed: false
    loginWithEmailAllowed: true
```

Apply it:

```bash
kubectl apply -f keycloak-realm.yaml
```

## Step 5: Create a Client

Create an OAuth2/OIDC client:

```yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakClient
metadata:
  name: my-app
  namespace: keycloak-operator
spec:
  realmRef:
    name: my-realm
  definition:
    clientId: my-app
    name: My Application
    enabled: true
    publicClient: false
    standardFlowEnabled: true
    directAccessGrantsEnabled: false
    redirectUris:
      - "https://my-app.example.com/callback"
    webOrigins:
      - "https://my-app.example.com"
  clientSecret:
    secretName: my-app-credentials
    key: clientSecret
```

Apply it:

```bash
kubectl apply -f keycloak-client.yaml
```

The operator will create a Kubernetes secret with the client credentials:

```bash
kubectl get secret my-app-credentials -n keycloak-operator -o yaml
```

## Step 6: Verify Resources

Check the status of all your Keycloak resources:

```bash
kubectl get keycloakinstances,keycloakrealms,keycloakclients -n keycloak-operator
```

## Next Steps

- Learn about [Helm Chart configuration](./helm.md)
- Explore all [Custom Resource Definitions](../crds.md)
- Set up a [local development environment](./kind.md)
