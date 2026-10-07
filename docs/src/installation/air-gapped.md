# Air-Gapped Installation

The operator only needs network access to the Kubernetes API and to the Keycloak instances it manages. Everything else can be mirrored once per release into an internal registry.

## Artefacts

Per release `vX.Y.Z`:

| Artefact | Location |
|----------|----------|
| Controller image (multi-arch) | `ghcr.io/hostzero-gmbh/keycloak-operator:vX.Y.Z` |
| Helm chart | `oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator:X.Y.Z` |
| CRDs (optional, if not installed via chart) | `keycloak-operator-crds.yaml` on the [GitHub release](https://github.com/Hostzero-GmbH/keycloak-operator/releases) |

All of them are public, no credentials needed on the connected side.

## Helm

### 1. Mirror

On a host with internet access and access to the internal registry:

```bash
export VERSION=0.13.1
export MIRROR=registry.example.internal/hostzero-gmbh

# Image, all architectures
skopeo copy --all \
  docker://ghcr.io/hostzero-gmbh/keycloak-operator:v${VERSION} \
  docker://${MIRROR}/keycloak-operator:v${VERSION}

# Chart
helm pull oci://ghcr.io/hostzero-gmbh/charts/keycloak-operator --version ${VERSION}
helm push keycloak-operator-${VERSION}.tgz oci://${MIRROR}/charts
```

For a sneakernet transfer use `skopeo copy --all docker://... dir:./keycloak-operator` on the connected side and `skopeo copy --all dir:./keycloak-operator docker://${MIRROR}/...` on the disconnected side; the chart tgz is a plain file.

### 2. Install from the mirror

```bash
helm install keycloak-operator oci://${MIRROR}/charts/keycloak-operator \
  --version ${VERSION} \
  --namespace keycloak-operator \
  --create-namespace \
  --set image.repository=${MIRROR}/keycloak-operator
```

The image tag defaults to the chart `appVersion`, so only the repository changes. Add `imagePullSecrets` in the values if the mirror requires authentication.

## OpenShift / OKD

The operator is part of the community catalog shipped with OpenShift (`registry.redhat.io/redhat/community-operator-index`), so the standard `oc-mirror` workflow applies. The bundle references the controller image by digest, which `oc-mirror` requires.

### 1. Mirror

```yaml
# imageset-config.yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/community-operator-index:v4.18
      packages:
        - name: hostzero-keycloak-operator
          channels:
            - name: stable
```

Match the catalog tag to your cluster version. Then:

```bash
# Connected side (or directly to the mirror registry if reachable)
oc mirror -c imageset-config.yaml file://keycloak-operator --v2

# Disconnected side
oc mirror -c imageset-config.yaml --from file://keycloak-operator docker://registry.example.internal --v2
```

### 2. Apply the generated resources

`oc-mirror` writes a `CatalogSource` and an `ImageDigestMirrorSet` to `working-dir/cluster-resources/`:

```bash
oc apply -f working-dir/cluster-resources/
```

The operator then shows up in the console under OperatorHub like any other catalog entry and can be installed from there or via a `Subscription`:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: hostzero-keycloak-operator
  namespace: openshift-operators
spec:
  channel: stable
  name: hostzero-keycloak-operator
  source: cs-community-operator-index-v4-18 # name from the generated CatalogSource
  sourceNamespace: openshift-marketplace
```

## Next Steps

Continue with the [Quick Start](./quick-start.md) to connect the operator to a Keycloak instance.
