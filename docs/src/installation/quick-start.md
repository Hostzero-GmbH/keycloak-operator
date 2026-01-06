# Quick Start

Get the Keycloak Operator running in minutes.

## Prerequisites

- Kubernetes cluster
- kubectl configured
- Helm 3

## Step 1: Install the Operator

```bash
helm install keycloak-operator oci://ghcr.io/hostzero/charts/keycloak-operator \
  --namespace keycloak-operator \
  --create-namespace
```

## Step 2: Create a Secret for Keycloak Credentials

```bash
kubectl create secret generic keycloak-admin \
  --from-literal=username=admin \
  --from-literal=password=admin
```

## Step 3: Create a KeycloakInstance

```yaml
# keycloak-instance.yaml
apiVersion: keycloak.hostzero.com/v1beta1
kind: KeycloakInstance
metadata:
  name: main
spec:
  baseUrl: https://keycloak.example.com
  credentials:
    secretRef:
      name: keycloak-admin
```

Apply it:

```bash
kubectl apply -f keycloak-instance.yaml
```

## Step 4: Create a Realm

```yaml
# my-realm.yaml
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
    displayName: My Application
```

Apply it:

```bash
kubectl apply -f my-realm.yaml
```

## Step 5: Verify

Check the status of your resources:

```bash
kubectl get keycloakinstances
kubectl get keycloakrealms
```

Both should show `Ready: true`.

## Next Steps

- [Create clients](../crds/keycloakclient.md) for your applications
- [Create users](../crds/keycloakuser.md) for authentication
- [Configure roles](../crds/keycloakrole.md) for authorization
