# Testing

The Keycloak Operator has two levels of testing:

1. **Unit Tests**: Fast, isolated tests using `envtest`
2. **End-to-End Tests**: Full cluster tests against Kind with Keycloak

## Unit Tests

Run unit tests with:

```bash
make test
```

Unit tests use the controller-runtime's `envtest` package to provide a lightweight Kubernetes API server. These don't require a real Keycloak instance.

### Coverage

```bash
make test
go tool cover -html=cover.out
```

## End-to-End Tests

E2E tests run against a full Kind cluster with the operator and Keycloak deployed.

### Understanding E2E Test Network Topology

E2E tests involve two different network perspectives:

1. **Operator's perspective** (inside the cluster): The operator connects to Keycloak using the in-cluster service URL (e.g., `http://keycloak.keycloak.svc.cluster.local`)
2. **Test's perspective** (your local machine): When running tests locally, you need port-forwarding to access Keycloak directly for certain tests (drift detection, cleanup verification)

```
┌─────────────────────────────────────────────────────────┐
│                     Kind Cluster                         │
│  ┌─────────────┐      ┌──────────────────┐             │
│  │  Operator   │──────│     Keycloak     │             │
│  │             │      │  (port 80/8080)  │             │
│  └─────────────┘      └────────┬─────────┘             │
│                                │                        │
└────────────────────────────────┼────────────────────────┘
                                 │ port-forward
                                 ▼
                    ┌────────────────────────┐
                    │   localhost:8080       │
                    │   (your machine)       │
                    └────────────────────────┘
```

### Running E2E Tests

**Recommended approach** (fully automated):

```bash
# Full setup: creates cluster, deploys operator and Keycloak, runs tests
make kind-all
make kind-test
```

The `kind-test` target runs `./hack/setup-kind.sh test-e2e`, which:
1. Sets up port-forwarding to Keycloak automatically
2. Configures environment variables
3. Runs the e2e test suite with a 30-minute timeout

**Development workflow** (for iterating on code changes):

```bash
# 1. Initial setup (only needed once)
make kind-all

# 2. In a separate terminal, start port-forward (keep this running)
make kind-port-forward

# 3. After making code changes, rebuild and redeploy the operator
make kind-redeploy

# 4. Run all e2e tests
make kind-test-run

# 5. Or run specific tests using TEST_RUN
make kind-test-run TEST_RUN=TestPreserveResourceAnnotation
```

The `kind-redeploy` target handles the full update cycle:
1. Rebuilds the Docker image (layer caching detects source changes automatically)
2. Removes old images from Kind nodes (avoids containerd tag caching)
3. Loads the new image into the Kind cluster
4. Restarts the operator deployment and waits for it to be ready

**Manual setup** (for full control):

```bash
# 1. Ensure cluster and operator are running
make kind-all

# 2. In a separate terminal, start port-forward
kubectl port-forward -n keycloak svc/keycloak 8080:80

# 3. Run tests with required environment variables
export USE_EXISTING_CLUSTER=true
export KEYCLOAK_URL="http://localhost:8080"                      # For test's direct Keycloak access
export KEYCLOAK_INTERNAL_URL="http://keycloak.keycloak.svc.cluster.local"  # For operator (inside cluster)
go test -v -timeout 30m ./test/e2e/...
```

> **Note**: Tests that require direct Keycloak access (drift detection, cleanup verification) will be **automatically skipped** if port-forward is not available. This allows running basic E2E tests without port-forwarding, while advanced tests require it.

### Quick Reference: Make Targets

| Target | Description |
|--------|-------------|
| `make kind-all` | Create cluster, build operator, deploy everything |
| `make kind-redeploy` | Rebuild, reload, and restart operator (for code changes) |
| `make kind-test` | Run all e2e tests with auto port-forward |
| `make kind-test-run` | Run e2e tests (port-forward must be running) |
| `make kind-test-run TEST_RUN=TestFoo` | Run specific test(s) matching pattern |
| `make kind-port-forward` | Start port-forward to Keycloak |
| `make kind-logs` | Tail operator logs |
| `make kind-status` | Show cluster and operator status |

### E2E Test Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `USE_EXISTING_CLUSTER` | Set to `true` to use current kubeconfig | `false` |
| `KEYCLOAK_INSTANCE_NAME` | Name of existing KeycloakInstance to use | (creates new) |
| `KEYCLOAK_INSTANCE_NAMESPACE` | Namespace of existing instance | `keycloak-operator-e2e` |
| `OPERATOR_NAMESPACE` | Namespace where operator is deployed | `keycloak-operator` |
| `KEYCLOAK_URL` | URL for test's direct Keycloak access (via port-forward) | `http://localhost:8080` |
| `KEYCLOAK_INTERNAL_URL` | URL operator uses to connect (in-cluster) | `http://keycloak.keycloak.svc.cluster.local` |
| `TEST_NAMESPACE` | Namespace for test resources | `keycloak-operator-e2e` |
| `KEEP_TEST_NAMESPACE` | Don't delete namespace after tests | `false` |

### Test Categories

| Category | Requires Port-Forward | Description |
|----------|----------------------|-------------|
| Basic CRUD | No | Create, update, delete resources via Kubernetes API |
| Status verification | No | Verify `.status.ready` and conditions |
| Drift detection | **Yes** | Tests that modify Keycloak directly and verify reconciliation |
| Cleanup verification | **Yes** | Tests that verify resources are deleted from Keycloak |
| Edge cases | Mixed | Some require direct access, some don't |

### Unit Test Example

```go
func TestRealmController_Reconcile(t *testing.T) {
    // Setup
    scheme := runtime.NewScheme()
    _ = keycloakv1beta1.AddToScheme(scheme)
    
    realm := &keycloakv1beta1.KeycloakRealm{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-realm",
            Namespace: "default",
        },
        Spec: keycloakv1beta1.KeycloakRealmSpec{
            InstanceRef: "test-instance",
        },
    }
    
    client := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(realm).
        Build()
    
    // Test reconciliation...
}
```

### E2E Test Example

```go
func TestKeycloakRealmE2E(t *testing.T) {
    skipIfNoCluster(t)
    
    realm := &keycloakv1beta1.KeycloakRealm{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "e2e-realm",
            Namespace: testNamespace,
        },
        Spec: keycloakv1beta1.KeycloakRealmSpec{
            InstanceRef: instanceName,
            Definition: rawJSON(`{"realm": "e2e-realm", "enabled": true}`),
        },
    }
    
    require.NoError(t, k8sClient.Create(ctx, realm))
    t.Cleanup(func() {
        k8sClient.Delete(ctx, realm)
    })
    
    // Wait for ready
    err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, 
        func(ctx context.Context) (bool, error) {
            updated := &keycloakv1beta1.KeycloakRealm{}
            if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(realm), updated); err != nil {
                return false, nil
            }
            return updated.Status.Ready, nil
        })
    require.NoError(t, err)
}

// Example: Test requiring direct Keycloak access (drift detection)
func TestDriftDetection(t *testing.T) {
    skipIfNoCluster(t)
    skipIfNoKeycloakAccess(t)  // Skips if port-forward not available
    
    // ... test that modifies Keycloak directly ...
}
```

## CI/CD

Tests run automatically in GitHub Actions:

- Unit tests on every PR
- E2E tests on merge to main

## Test Utilities

Common test utilities are in `test/e2e/suite_test.go`:

- `skipIfNoCluster(t)`: Skip test if `USE_EXISTING_CLUSTER` is not set
- `skipIfNoKeycloakAccess(t)`: Skip test if direct Keycloak access (port-forward) is unavailable
- `getInternalKeycloakClient(t)`: Create authenticated Keycloak client for direct API access
- `rawJSON(s string)`: Create `runtime.RawExtension` from JSON string
- `canConnectToKeycloak()`: Check if direct Keycloak connection is available
