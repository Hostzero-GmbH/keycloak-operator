package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// requiredContext is the only Kubernetes context that E2E tests are allowed to run against.
// This prevents accidentally running tests against production clusters.
const requiredContext = "kind-keycloak-operator-dev"

var (
	testNamespace       string
	keycloakInstanceRef string
	timeout             = 30 * time.Second
	interval            = 1 * time.Second
)

var (
	k8sClient client.Client
	ctx       = context.Background()
)

func init() {
	// Allow configuring test namespace via environment
	testNamespace = os.Getenv("TEST_NAMESPACE")
	if testNamespace == "" {
		testNamespace = "keycloak-operator-e2e"
	}

	// Allow using existing KeycloakInstance
	keycloakInstanceRef = os.Getenv("KEYCLOAK_INSTANCE_NAME")
}

func TestMain(m *testing.M) {
	// Setup
	if err := setupSuite(); err != nil {
		fmt.Printf("Failed to setup test suite: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown
	teardownSuite()

	os.Exit(code)
}

func setupSuite() error {
	// Validate we're running against the correct context
	if err := validateKubeContext(); err != nil {
		return err
	}

	// Add scheme
	if err := keycloakv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	// Get config
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Create client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// validateKubeContext ensures tests only run against the allowed context
func validateKubeContext() error {
	// Load kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	currentContext := rawConfig.CurrentContext
	if currentContext != requiredContext {
		return fmt.Errorf(
			"E2E tests can only run against context %q, but current context is %q.\n"+
				"Switch context with: kubectl config use-context %s",
			requiredContext, currentContext, requiredContext)
	}

	// Also verify the context exists and is valid
	if _, exists := rawConfig.Contexts[currentContext]; !exists {
		return fmt.Errorf("context %q not found in kubeconfig", currentContext)
	}

	fmt.Printf("✓ Running E2E tests against context: %s\n", currentContext)
	return nil
}

// getCurrentContext returns the current kubectl context name
func getCurrentContext() string {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func teardownSuite() {
	// Only delete namespace if we created it (not using existing)
	if os.Getenv("KEEP_TEST_NAMESPACE") == "" {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		k8sClient.Delete(ctx, ns)
	}
}

// skipIfNoCluster skips the test if USE_EXISTING_CLUSTER is not set
func skipIfNoCluster(t *testing.T) {
	if os.Getenv("USE_EXISTING_CLUSTER") != "true" {
		t.Skip("Skipping e2e test - set USE_EXISTING_CLUSTER=true to run against a real cluster")
	}
}

// skipIfNoKeycloakAccess skips the test if direct Keycloak access is unavailable.
// This is used for tests that need to interact with Keycloak directly (not through CRs),
// such as drift detection tests or cleanup verification tests.
// These tests require port-forwarding to be set up (kubectl port-forward svc/keycloak 8080:80).
func skipIfNoKeycloakAccess(t *testing.T) {
	if !canConnectToKeycloak() {
		t.Skip("Skipping test - direct Keycloak access unavailable. Set up port-forwarding: kubectl port-forward -n keycloak svc/keycloak 8080:80")
	}
}

// getOrCreateInstance returns the KeycloakInstance name and namespace to use for tests
func getOrCreateInstance(t *testing.T) (string, string) {
	// Use existing instance if configured
	if keycloakInstanceRef != "" {
		instanceNS := os.Getenv("KEYCLOAK_INSTANCE_NAMESPACE")
		if instanceNS == "" {
			instanceNS = testNamespace
		}
		// Verify the instance exists
		instance := &keycloakv1beta1.KeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      keycloakInstanceRef,
			Namespace: instanceNS,
		}, instance)
		require.NoError(t, err, "Referenced KeycloakInstance not found")
		return keycloakInstanceRef, instanceNS
	}

	// Create a new instance for the test
	return createTestInstance(t), testNamespace
}

func createTestInstance(t *testing.T) string {
	// Create credentials secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keycloak-credentials",
			Namespace: testNamespace,
		},
		StringData: map[string]string{
			"username": "admin",
			"password": "admin",
		},
	}
	err := k8sClient.Create(ctx, secret)
	if err != nil && !errors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	// Determine Keycloak URL for the operator (always use in-cluster URL)
	// KEYCLOAK_INTERNAL_URL is what the operator uses (must be reachable from inside the cluster)
	// KEYCLOAK_URL is what tests use for direct access (can be port-forwarded localhost)
	keycloakInternalURL := os.Getenv("KEYCLOAK_INTERNAL_URL")
	if keycloakInternalURL == "" {
		// Default to in-cluster service URL
		keycloakInternalURL = "http://keycloak.keycloak.svc.cluster.local"
	}

	// Create KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: keycloakInternalURL,
			Credentials: keycloakv1beta1.CredentialsSpec{
				SecretRef: keycloakv1beta1.SecretRefSpec{
					Name: "keycloak-credentials",
				},
			},
		},
	}
	err = k8sClient.Create(ctx, instance)
	if err != nil && !errors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	// Wait for instance to be ready
	err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakInstance{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakInstance did not become ready")

	return instance.Name
}

// createTestRealm creates a test realm and returns its name
func createTestRealm(t *testing.T, instanceName, instanceNS, suffix string) string {
	realmName := fmt.Sprintf("test-realm-%s-%d", suffix, time.Now().UnixNano())
	realm := &keycloakv1beta1.KeycloakRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakRealmSpec{
			InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
			Definition: rawJSON(fmt.Sprintf(`{
				"realm": "%s",
				"enabled": true
			}`, realmName)),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, realm))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, realm)
	})

	// Wait for realm to be ready
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakRealm{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      realm.Name,
			Namespace: realm.Namespace,
		}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakRealm did not become ready")
	return realmName
}

// createTestRealmWithOrganizations creates a test realm with organizations feature enabled (required for Keycloak 26+)
func createTestRealmWithOrganizations(t *testing.T, instanceName, instanceNS, suffix string) string {
	realmName := fmt.Sprintf("test-realm-%s-%d", suffix, time.Now().UnixNano())
	realm := &keycloakv1beta1.KeycloakRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakRealmSpec{
			InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
			Definition: rawJSON(fmt.Sprintf(`{
				"realm": "%s",
				"enabled": true,
				"organizationsEnabled": true
			}`, realmName)),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, realm))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, realm)
	})

	// Wait for realm to be ready
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakRealm{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      realm.Name,
			Namespace: realm.Namespace,
		}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakRealm with organizations did not become ready")
	return realmName
}

// waitForReady waits for a resource to become ready
func waitForReady(t *testing.T, name, namespace string, obj client.Object, checkReady func() bool) {
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, obj); err != nil {
			return false, nil
		}
		return checkReady(), nil
	})
	require.NoError(t, err, "Resource did not become ready: %s/%s", namespace, name)
}

// waitForCondition waits for a specific condition
func waitForCondition(t *testing.T, name, namespace string, obj client.Object, condition func() bool, description string) {
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, obj); err != nil {
			return false, nil
		}
		return condition(), nil
	})
	require.NoError(t, err, "Condition not met: %s for %s/%s", description, namespace, name)
}

// getInternalKeycloakClient returns our internal Keycloak API client for testing
func getInternalKeycloakClient(t *testing.T) *keycloak.Client {
	keycloakURL := os.Getenv("KEYCLOAK_URL")
	if keycloakURL == "" {
		keycloakURL = "http://keycloak.keycloak.svc.cluster.local"
	}
	log := ctrl.Log.WithName("test")
	kc := keycloak.NewClient(keycloak.Config{
		BaseURL:  keycloakURL,
		Realm:    "master",
		Username: "admin",
		Password: "admin",
	}, log)
	return kc
}

// canConnectToKeycloak tests if we can connect to Keycloak from the test environment
func canConnectToKeycloak() bool {
	keycloakURL := os.Getenv("KEYCLOAK_URL")
	if keycloakURL == "" {
		keycloakURL = "http://keycloak.keycloak.svc.cluster.local"
	}
	log := ctrl.Log.WithName("test")
	kc := keycloak.NewClient(keycloak.Config{
		BaseURL:  keycloakURL,
		Realm:    "master",
		Username: "admin",
		Password: "admin",
	}, log)
	err := kc.Ping(ctx)
	return err == nil
}

// Helper function to run kubectl commands
func kubectl(args ...string) error {
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Helper function to get project root
func projectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// Helper to create raw JSON extension
func rawJSON(s string) runtime.RawExtension {
	return runtime.RawExtension{Raw: []byte(s)}
}

// getSecretKeys returns the keys in a secret's data
func getSecretKeys(secret *corev1.Secret) []string {
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	return keys
}
