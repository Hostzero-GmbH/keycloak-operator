# Testing

Guide to running tests.

## Unit Tests

```bash
make test
```

This runs all unit tests with coverage.

## Integration Tests

Integration tests require a running Keycloak instance:

```bash
# Start Kind cluster with Keycloak
make kind-create

# Run integration tests
USE_EXISTING_CLUSTER=true go test -v ./internal/controller/... -tags=integration
```

## End-to-End Tests

### Against Existing Cluster

```bash
make test-e2e
```

### With Kind Management

```bash
# Full cycle: create cluster, run tests, cleanup
make test-e2e-kind
```

### Specific Tests

```bash
# Run specific test
go test -v ./test/e2e/... -run TestKeycloakInstance

# Run with verbose output
go test -v ./test/e2e/... -ginkgo.v
```

## Test Structure

```
test/
└── e2e/
    ├── suite_test.go          # Test suite setup
    ├── instance_test.go       # KeycloakInstance tests
    ├── realm_test.go          # KeycloakRealm tests
    ├── client_test.go         # KeycloakClient tests
    ├── user_test.go           # KeycloakUser tests
    └── ...
```

## Writing Tests

### Unit Test Example

```go
func TestMyFunction(t *testing.T) {
    result := MyFunction("input")
    assert.Equal(t, "expected", result)
}
```

### E2E Test Example

```go
var _ = Describe("KeycloakRealm", func() {
    It("should create realm", func() {
        realm := &keycloakv1beta1.KeycloakRealm{
            // ...
        }
        Expect(k8sClient.Create(ctx, realm)).Should(Succeed())
        
        Eventually(func() bool {
            // check status
        }).Should(BeTrue())
    })
})
```

## Coverage

Generate coverage report:

```bash
make test
go tool cover -html=cover.out
```
