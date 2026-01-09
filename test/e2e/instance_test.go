package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakInstanceE2E(t *testing.T) {
	skipIfNoCluster(t)

	t.Run("BasicInstance", func(t *testing.T) {
		instanceName, instanceNS := getOrCreateInstance(t)
		require.NotEmpty(t, instanceName)

		// Verify instance is ready
		instance := &keycloakv1beta1.KeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      instanceName,
			Namespace: instanceNS,
		}, instance)
		require.NoError(t, err)
		require.True(t, instance.Status.Ready, "Instance should be ready")
		t.Logf("KeycloakInstance %s/%s is ready, version: %s", instanceNS, instanceName, instance.Status.Version)
	})

	t.Run("InvalidCredentials", func(t *testing.T) {
		// Create a secret with invalid credentials
		secretName := fmt.Sprintf("invalid-creds-%d", time.Now().UnixNano())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"username": "wronguser",
				"password": "wrongpassword",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, secret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, secret)
		})

		// Create instance with invalid credentials
		instanceName := fmt.Sprintf("invalid-instance-%d", time.Now().UnixNano())
		keycloakURL := getKeycloakURL()

		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: keycloakURL,
				Credentials: keycloakv1beta1.CredentialsSpec{
					SecretRef: keycloakv1beta1.SecretRefSpec{
						Name: secretName,
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		// Wait and verify the instance is NOT ready (should have error status)
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      instanceName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Instance with invalid credentials should not be ready")
		t.Logf("Instance correctly failed with invalid credentials, message: %s", updated.Status.Message)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		// Create credentials secret
		secretName := fmt.Sprintf("creds-invalid-url-%d", time.Now().UnixNano())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "admin",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, secret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, secret)
		})

		// Create instance with invalid URL
		instanceName := fmt.Sprintf("invalid-url-instance-%d", time.Now().UnixNano())
		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: "http://non-existent-keycloak.invalid:8080",
				Credentials: keycloakv1beta1.CredentialsSpec{
					SecretRef: keycloakv1beta1.SecretRefSpec{
						Name: secretName,
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		// Wait and verify the instance is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      instanceName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Instance with invalid URL should not be ready")
		t.Logf("Instance correctly failed with invalid URL, message: %s", updated.Status.Message)
	})

	t.Run("MissingCredentialsSecret", func(t *testing.T) {
		instanceName := fmt.Sprintf("missing-secret-instance-%d", time.Now().UnixNano())
		keycloakURL := getKeycloakURL()

		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: keycloakURL,
				Credentials: keycloakv1beta1.CredentialsSpec{
					SecretRef: keycloakv1beta1.SecretRefSpec{
						Name: "non-existent-secret",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		// Wait and verify the instance is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      instanceName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Instance with missing secret should not be ready")
		t.Logf("Instance correctly failed with missing secret, message: %s", updated.Status.Message)
	})
}

// getKeycloakURL returns the Keycloak URL for tests
func getKeycloakURL() string {
	if url := os.Getenv("KEYCLOAK_URL"); url != "" {
		return url
	}
	return "http://keycloak.keycloak.svc.cluster.local"
}
