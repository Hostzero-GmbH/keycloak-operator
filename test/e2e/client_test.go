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
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name: clientName + "-secret",
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
				// No ClientSecretRef specified - public clients don't have secrets
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
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name:            clientName + "-secret",
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

	t.Run("ClientWithPreExistingSecret", func(t *testing.T) {
		// Create a pre-existing secret with known values
		clientName := fmt.Sprintf("preexisting-secret-client-%d", time.Now().UnixNano())
		secretName := clientName + "-secret"
		knownSecret := "my-predefined-secret-value"

		// Create the secret first
		preExistingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"client-id":     []byte(clientName),
				"client-secret": []byte(knownSecret),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, preExistingSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, preExistingSecret)
		})

		// Create KeycloakClient with create=false (strict mode)
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Pre-existing Secret Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true
		}`, clientName))
		createFalse := false
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name:   secretName,
					Create: &createFalse,
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
		require.NoError(t, err, "Client with pre-existing secret did not become ready")
		t.Logf("Client with pre-existing secret %s is ready", clientName)

		// Verify the secret still has the original value (not overwritten)
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: testNamespace,
		}, secret)
		require.NoError(t, err)
		require.Equal(t, knownSecret, string(secret.Data["client-secret"]), "Pre-existing secret should not be overwritten")
		t.Log("Verified: Pre-existing secret value was preserved")
	})

	t.Run("ClientWithPreExistingSecretCustomKeys", func(t *testing.T) {
		// Create a pre-existing secret with custom key names
		clientName := fmt.Sprintf("preexisting-customkeys-client-%d", time.Now().UnixNano())
		secretName := clientName + "-secret"
		knownSecret := "my-custom-key-secret-value"
		customIdKey := "MY_CLIENT_ID"
		customSecretKey := "MY_CLIENT_SECRET"

		// Create the secret first with custom keys
		preExistingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				customIdKey:     []byte(clientName),
				customSecretKey: []byte(knownSecret),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, preExistingSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, preExistingSecret)
		})

		// Create KeycloakClient with create=false and custom keys
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Pre-existing Secret Custom Keys Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true
		}`, clientName))
		createFalse := false
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name:            secretName,
					ClientIdKey:     &customIdKey,
					ClientSecretKey: &customSecretKey,
					Create:          &createFalse,
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
		require.NoError(t, err, "Client with pre-existing secret (custom keys) did not become ready")
		t.Logf("Client with pre-existing secret (custom keys) %s is ready", clientName)
	})

	t.Run("ClientWithMissingSecretStrictMode", func(t *testing.T) {
		// Create KeycloakClient with create=false but no pre-existing secret
		clientName := fmt.Sprintf("missing-secret-strict-client-%d", time.Now().UnixNano())
		secretName := clientName + "-nonexistent-secret"

		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Missing Secret Strict Mode Client",
			"enabled": true,
			"publicClient": false
		}`, clientName))
		createFalse := false
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name:   secretName,
					Create: &createFalse,
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait and verify the client is NOT ready with SecretError
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakClient{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      clientName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Client with missing secret (strict mode) should not be ready")
		require.Equal(t, "SecretError", updated.Status.Status, "Status should be SecretError")
		t.Logf("Client correctly failed with missing secret in strict mode, message: %s", updated.Status.Message)
	})

	t.Run("ClientSecretAutoCreate", func(t *testing.T) {
		// Create KeycloakClient with create=true (default) - secret should be auto-created
		clientName := fmt.Sprintf("autocreate-secret-client-%d", time.Now().UnixNano())
		secretName := clientName + "-secret"

		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Auto Create Secret Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true
		}`, clientName))
		// create is true by default, so we don't need to set it
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name: secretName,
					// Create defaults to true
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
		require.NoError(t, err, "Client with auto-create secret did not become ready")

		// Verify secret was automatically created
		secret := &corev1.Secret{}
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      secretName,
				Namespace: testNamespace,
			}, secret); err != nil {
				return false, nil
			}
			return true, nil
		})
		require.NoError(t, err, "Auto-created secret was not found")
		require.Contains(t, secret.Data, "client-id", "Secret should contain client-id")
		require.Contains(t, secret.Data, "client-secret", "Secret should contain client-secret")
		require.NotEmpty(t, secret.Data["client-secret"], "client-secret should not be empty")
		t.Logf("Auto-created secret %s with keys: %v", secretName, getSecretKeys(secret))
	})

	t.Run("ClientSecretMissingKey", func(t *testing.T) {
		// Create a secret without the expected key
		clientName := fmt.Sprintf("missing-key-client-%d", time.Now().UnixNano())
		secretName := clientName + "-secret"

		// Create secret with wrong key name
		preExistingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"wrong-key": []byte("some-value"),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, preExistingSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, preExistingSecret)
		})

		// Create KeycloakClient expecting default key "client-secret"
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Missing Key Client",
			"enabled": true,
			"publicClient": false
		}`, clientName))
		createFalse := false
		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
				ClientSecretRef: &keycloakv1beta1.ClientSecretRefSpec{
					Name:   secretName,
					Create: &createFalse,
				},
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
		require.False(t, updated.Status.Ready, "Client with missing key should not be ready")
		require.Equal(t, "SecretError", updated.Status.Status, "Status should be SecretError")
		require.Contains(t, updated.Status.Message, "not found in secret", "Message should mention missing key")
		t.Logf("Client correctly failed with missing key, message: %s", updated.Status.Message)
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
