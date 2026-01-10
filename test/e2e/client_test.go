package e2e

import (
	"context"
	"fmt"
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

func TestKeycloakClientE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "client")

	t.Run("ConfidentialClient", func(t *testing.T) {
		// Create confidential client with service account
		clientName := fmt.Sprintf("confidential-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Confidential Client",
			"enabled": true,
			"publicClient": false,
			"standardFlowEnabled": true,
			"serviceAccountsEnabled": true,
			"directAccessGrantsEnabled": true
		}`, clientName))
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecret: &keycloakv1beta1.ClientSecretSpec{
					SecretName: clientName + "-secret",
				},
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
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Confidential client did not become ready")
		t.Logf("Confidential client %s is ready", clientName)

		// Verify secret was created with credentials
		secret := &corev1.Secret{}
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientName + "-secret",
				Namespace: testNamespace,
			}, secret); err != nil {
				return false, nil
			}
			return true, nil
		})
		require.NoError(t, err, "Client secret was not created")
		require.Contains(t, secret.Data, "client-id", "Secret should contain client-id")
		require.Contains(t, secret.Data, "client-secret", "Secret should contain client-secret")
		require.NotEmpty(t, secret.Data["client-secret"], "client-secret should not be empty")
		t.Logf("Confidential client secret created with keys: %v", getSecretKeys(secret))
	})

	t.Run("PublicClient", func(t *testing.T) {
		// Create public client (no secret should be generated)
		clientName := fmt.Sprintf("public-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Public Client",
			"enabled": true,
			"publicClient": true,
			"standardFlowEnabled": true,
			"directAccessGrantsEnabled": true,
			"redirectUris": ["http://localhost:8080/*"]
		}`, clientName))
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				// No ClientSecret specified - public clients don't have secrets
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
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Public client did not become ready")
		t.Logf("Public client %s is ready", clientName)

		// Verify NO secret was created for public client
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      clientName + "-secret",
			Namespace: testNamespace,
		}, secret)
		require.True(t, errors.IsNotFound(err), "Public client should NOT have a secret created")
		t.Log("Verified: No secret created for public client")
	})

	t.Run("BearerOnlyClient", func(t *testing.T) {
		// Create bearer-only client (for backend services)
		clientName := fmt.Sprintf("bearer-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Bearer Only Client",
			"enabled": true,
			"publicClient": false,
			"bearerOnly": true
		}`, clientName))
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				// Bearer-only clients don't need secrets stored
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
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Bearer-only client did not become ready")
		t.Logf("Bearer-only client %s is ready", clientName)
	})

	t.Run("ClientWithCustomSecretKeys", func(t *testing.T) {
		// Create client with custom secret key names
		clientName := fmt.Sprintf("custom-keys-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Custom Keys Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true
		}`, clientName))
		customIdKey := "OIDC_CLIENT_ID"
		customSecretKey := "OIDC_CLIENT_SECRET"
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecret: &keycloakv1beta1.ClientSecretSpec{
					SecretName:      clientName + "-secret",
					ClientIdKey:     &customIdKey,
					ClientSecretKey: &customSecretKey,
				},
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
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Client with custom keys did not become ready")

		// Verify secret has custom key names
		secret := &corev1.Secret{}
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientName + "-secret",
				Namespace: testNamespace,
			}, secret); err != nil {
				return false, nil
			}
			return true, nil
		})
		require.NoError(t, err, "Client secret was not created")
		require.Contains(t, secret.Data, "OIDC_CLIENT_ID", "Secret should contain custom client-id key")
		require.Contains(t, secret.Data, "OIDC_CLIENT_SECRET", "Secret should contain custom client-secret key")
		t.Logf("Custom keys client secret created with keys: %v", getSecretKeys(secret))
	})

	t.Run("InvalidRealmRef", func(t *testing.T) {
		clientName := fmt.Sprintf("invalid-realm-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"enabled": true
		}`, clientName))
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: "non-existent-realm"},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait and verify the client is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakClient{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      clientName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Client with invalid realm ref should not be ready")
		t.Logf("Client correctly failed with invalid realm ref, message: %s", updated.Status.Message)
	})

	t.Run("DefaultsToResourceName", func(t *testing.T) {
		// When no definition is provided, the controller uses the resource name as clientId
		clientName := fmt.Sprintf("no-def-client-%d", time.Now().UnixNano())
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				// No Definition provided - should default clientId to resource name
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait for client to become ready (controller defaults clientId to resource name)
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
		require.NoError(t, err, "Client with no definition should default to resource name as clientId")
		t.Logf("Client correctly created using resource name as clientId")
	})

	t.Run("ReconcileAfterManualDeletion", func(t *testing.T) {
		// Skip if not running in-cluster or without port-forward
		if !canConnectToKeycloak() {
			t.Skip("Skipping reconcile test - cannot connect to Keycloak from test environment")
		}

		// Create a client
		clientName := fmt.Sprintf("reconcile-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Reconcile Test Client",
			"enabled": true,
			"publicClient": false
		}`, clientName))
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait for client to be ready
		var clientUUID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			if updated.Status.Ready {
				clientUUID = updated.Status.ClientUUID
				return true, nil
			}
			return false, nil
		})
		require.NoError(t, err, "Client did not become ready")
		require.NotEmpty(t, clientUUID, "Client should have a UUID")
		t.Log("Client is ready, now deleting it directly from Keycloak")

		// Delete the client directly from Keycloak using its internal ID
		kc := getInternalKeycloakClient(t)
		err = kc.DeleteClient(ctx, realmName, clientUUID)
		require.NoError(t, err, "Failed to delete client from Keycloak")
		t.Log("Client deleted from Keycloak, waiting for reconciliation")

		// Trigger reconciliation by updating the CR
		updated := &keycloakv1beta1.KeycloakClient{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcClient.Name,
			Namespace: kcClient.Namespace,
		}, updated)
		require.NoError(t, err)

		// Add an annotation to trigger reconciliation
		if updated.Annotations == nil {
			updated.Annotations = make(map[string]string)
		}
		updated.Annotations["test/reconcile-trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
		err = k8sClient.Update(ctx, updated)
		require.NoError(t, err)

		// Wait for the client to be recreated
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			// Check if client exists in Keycloak by searching for it
			clients, err := kc.GetClients(ctx, realmName, map[string]string{
				"clientId": clientName,
			})
			if err != nil {
				return false, nil
			}
			return len(clients) > 0, nil
		})
		require.NoError(t, err, "Client was not recreated in Keycloak after deletion")
		t.Log("Client was successfully reconciled (recreated) after manual deletion")
	})
}
