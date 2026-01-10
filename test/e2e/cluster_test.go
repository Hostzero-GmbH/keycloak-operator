package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TestClusterKeycloakInstanceE2E tests cluster-scoped Keycloak instance management
func TestClusterKeycloakInstanceE2E(t *testing.T) {
	skipIfNoCluster(t)

	t.Run("CreateClusterInstance", func(t *testing.T) {
		instanceName := fmt.Sprintf("cluster-instance-%d", time.Now().UnixNano())

		// Create credentials secret in a specific namespace
		secretNS := testNamespace
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-creds", instanceName),
				Namespace: secretNS,
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

		// Determine Keycloak URL for the operator (always use in-cluster URL)
		keycloakInternalURL := os.Getenv("KEYCLOAK_INTERNAL_URL")
		if keycloakInternalURL == "" {
			keycloakInternalURL = "http://keycloak.keycloak.svc.cluster.local"
		}

		// Create ClusterKeycloakInstance
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: instanceName,
			},
			Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
				BaseUrl: keycloakInternalURL,
				Credentials: keycloakv1beta1.ClusterCredentialsSpec{
					SecretRef: keycloakv1beta1.ClusterSecretRefSpec{
						Name:      secret.Name,
						Namespace: secretNS,
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterInstance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clusterInstance)
		})

		// Wait for instance to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "ClusterKeycloakInstance did not become ready")

		// Verify status
		updatedInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: instanceName}, updatedInstance))
		require.Equal(t, "Ready", updatedInstance.Status.Status)
		require.NotEmpty(t, updatedInstance.Status.Version, "Should have detected Keycloak version")
	})
}

// TestClusterKeycloakRealmE2E tests cluster-scoped realm management
func TestClusterKeycloakRealmE2E(t *testing.T) {
	skipIfNoCluster(t)

	// First create a ClusterKeycloakInstance
	clusterInstanceName := getOrCreateClusterInstance(t)

	t.Run("CreateClusterRealm", func(t *testing.T) {
		realmName := fmt.Sprintf("cluster-realm-%d", time.Now().UnixNano())

		realmDef := rawJSON(fmt.Sprintf(`{
			"realm": "%s",
			"enabled": true,
			"displayName": "Cluster Realm Test"
		}`, realmName))

		clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name: realmName,
			},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{
					Name: clusterInstanceName,
				},
				Definition: realmDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterRealm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clusterRealm)
		})

		// Wait for realm to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm did not become ready")

		// Verify status
		updatedRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updatedRealm))
		require.Equal(t, "Ready", updatedRealm.Status.Status)
		require.Equal(t, realmName, updatedRealm.Status.RealmName)
		require.NotEmpty(t, updatedRealm.Status.ResourcePath)
	})

	t.Run("ClusterRealmWithNamespacedInstance", func(t *testing.T) {
		// Get or create a namespaced instance
		instanceName, instanceNS := getOrCreateInstance(t)

		realmName := fmt.Sprintf("cluster-realm-ns-%d", time.Now().UnixNano())

		realmDef := rawJSON(fmt.Sprintf(`{
			"realm": "%s",
			"enabled": true
		}`, realmName))

		clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name: realmName,
			},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.NamespacedRef{
					Name:      instanceName,
					Namespace: instanceNS,
				},
				Definition: realmDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterRealm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clusterRealm)
		})

		// Wait for realm to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm with namespaced instance did not become ready")
	})
}

// TestClusterRealmCrossNamespaceE2E tests that resources in different namespaces can use cluster realms
func TestClusterRealmCrossNamespaceE2E(t *testing.T) {
	skipIfNoCluster(t)

	clusterInstanceName := getOrCreateClusterInstance(t)

	// Create a ClusterKeycloakRealm
	realmName := fmt.Sprintf("cross-ns-realm-%d", time.Now().UnixNano())
	realmDef := rawJSON(fmt.Sprintf(`{
		"realm": "%s",
		"enabled": true
	}`, realmName))

	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name: realmName,
		},
		Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
			ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{
				Name: clusterInstanceName,
			},
			Definition: realmDef,
		},
	}
	require.NoError(t, k8sClient.Create(ctx, clusterRealm))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, clusterRealm)
	})

	// Wait for realm to be ready
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.ClusterKeycloakRealm{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "ClusterKeycloakRealm did not become ready")

	t.Run("ClientInDifferentNamespace", func(t *testing.T) {
		// Create a client in test namespace using the cluster realm
		clientName := fmt.Sprintf("cross-ns-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"enabled": true,
			"protocol": "openid-connect",
			"publicClient": true
		}`, clientName))

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{
					Name: realmName,
				},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait for client to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientName,
				Namespace: testNamespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakClient using clusterRealmRef did not become ready")

		// Verify status shows cluster realm ref
		updatedClient := &keycloakv1beta1.KeycloakClient{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      clientName,
			Namespace: testNamespace,
		}, updatedClient))
		require.True(t, updatedClient.Status.Ready)
	})

	t.Run("UserInDifferentNamespace", func(t *testing.T) {
		// Create a user in test namespace using the cluster realm
		userName := fmt.Sprintf("cross-ns-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"email": "%s@example.com",
			"enabled": true
		}`, userName, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{
					Name: realmName,
				},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      userName,
				Namespace: testNamespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser using clusterRealmRef did not become ready")
	})
}

// TestClusterResourceCleanup tests that cluster resources are properly cleaned up
func TestClusterResourceCleanup(t *testing.T) {
	skipIfNoCluster(t)

	clusterInstanceName := getOrCreateClusterInstance(t)

	t.Run("ClusterRealmDeletion", func(t *testing.T) {
		realmName := fmt.Sprintf("cleanup-realm-%d", time.Now().UnixNano())
		realmDef := rawJSON(fmt.Sprintf(`{
			"realm": "%s",
			"enabled": true
		}`, realmName))

		clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name: realmName,
			},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{
					Name: clusterInstanceName,
				},
				Definition: realmDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterRealm))

		// Wait for realm to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete the realm
		require.NoError(t, k8sClient.Delete(ctx, clusterRealm))

		// Wait for realm to be fully deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, &keycloakv1beta1.ClusterKeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm should be deleted")
	})
}

// getOrCreateClusterInstance returns or creates a ClusterKeycloakInstance for testing
func getOrCreateClusterInstance(t *testing.T) string {
	instanceName := os.Getenv("CLUSTER_KEYCLOAK_INSTANCE_NAME")
	if instanceName != "" {
		// Verify the instance exists
		instance := &keycloakv1beta1.ClusterKeycloakInstance{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName}, instance)
		require.NoError(t, err, "Referenced ClusterKeycloakInstance not found")
		return instanceName
	}

	// Create a new cluster instance for the test
	instanceName = fmt.Sprintf("e2e-cluster-instance-%d", time.Now().UnixNano())

	// Create credentials secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-creds", instanceName),
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
	t.Cleanup(func() {
		k8sClient.Delete(ctx, secret)
	})

	// Determine Keycloak URL for the operator (always use in-cluster URL)
	keycloakInternalURL := os.Getenv("KEYCLOAK_INTERNAL_URL")
	if keycloakInternalURL == "" {
		keycloakInternalURL = "http://keycloak.keycloak.svc.cluster.local"
	}

	// Create ClusterKeycloakInstance
	clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: instanceName,
		},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			BaseUrl: keycloakInternalURL,
			Credentials: keycloakv1beta1.ClusterCredentialsSpec{
				SecretRef: keycloakv1beta1.ClusterSecretRefSpec{
					Name:      secret.Name,
					Namespace: testNamespace,
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, clusterInstance))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, clusterInstance)
	})

	// Wait for instance to be ready
	err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "ClusterKeycloakInstance did not become ready")

	return instanceName
}
