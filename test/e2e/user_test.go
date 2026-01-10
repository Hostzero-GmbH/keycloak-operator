package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestKeycloakUserE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "user")

	t.Run("BasicUser", func(t *testing.T) {
		// Create user
		userName := fmt.Sprintf("test-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"email": "%s@example.com",
			"firstName": "Test",
			"lastName": "User",
			"enabled": true
		}`, userName, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser did not become ready")
		t.Logf("KeycloakUser %s is ready", userName)
	})

	t.Run("InvalidRealmRef", func(t *testing.T) {
		userName := fmt.Sprintf("invalid-realm-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"enabled": true
		}`, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: "non-existent-realm"},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait and verify the user is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakUser{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      userName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "User with invalid realm ref should not be ready")
		t.Logf("User correctly failed with invalid realm ref, message: %s", updated.Status.Message)
	})

	t.Run("InvalidUserDefinition", func(t *testing.T) {
		userName := fmt.Sprintf("invalid-def-user-%d", time.Now().UnixNano())
		// Valid JSON but with empty username which Keycloak won't accept
		userDef := rawJSON(`{"username": "", "enabled": true}`)
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait and verify the user is NOT ready (empty username should fail)
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakUser{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      userName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "User with empty username should not be ready")
		t.Logf("User correctly failed with invalid definition, message: %s", updated.Status.Message)
	})

	t.Run("DuplicateUsername", func(t *testing.T) {
		// Note: The controller handles duplicate usernames by updating the existing user
		// rather than failing. This test verifies that behavior - both CRs pointing to
		// the same username will both become ready (managing the same Keycloak user).

		// Create first user
		userName := fmt.Sprintf("dup-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"email": "%s@example.com",
			"enabled": true
		}`, userName, userName))
		kcUser1 := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName + "-1",
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser1))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser1)
		})

		// Wait for first user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser1.Name,
				Namespace: kcUser1.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "First user did not become ready")

		// Get the first user's Keycloak ID
		firstUser := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser1.Name,
			Namespace: kcUser1.Namespace,
		}, firstUser)
		require.NoError(t, err)
		firstUserID := firstUser.Status.UserID

		// Create second user CR with same username
		kcUser2 := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName + "-2",
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef, // Same username
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser2))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser2)
		})

		// Wait for second user to be ready (it updates the same Keycloak user)
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser2.Name,
				Namespace: kcUser2.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Second user should become ready (updates same Keycloak user)")

		// Verify both CRs point to the same Keycloak user ID
		secondUser := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser2.Name,
			Namespace: kcUser2.Namespace,
		}, secondUser)
		require.NoError(t, err)
		require.Equal(t, firstUserID, secondUser.Status.UserID, "Both CRs should manage the same Keycloak user")
		t.Logf("Both user CRs managing same Keycloak user ID: %s", firstUserID)
	})

	t.Run("ReconcileAfterManualDeletion", func(t *testing.T) {
		// Skip if not running in-cluster or without port-forward
		if !canConnectToKeycloak() {
			t.Skip("Skipping reconcile test - cannot connect to Keycloak from test environment")
		}

		// Create a user
		userName := fmt.Sprintf("reconcile-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"email": "%s@example.com",
			"firstName": "Reconcile",
			"lastName": "Test",
			"enabled": true
		}`, userName, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready and get user ID
		var userID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			if updated.Status.Ready {
				userID = updated.Status.UserID
				return true, nil
			}
			return false, nil
		})
		require.NoError(t, err, "User did not become ready")
		require.NotEmpty(t, userID, "User should have a user ID")
		t.Log("User is ready, now deleting it directly from Keycloak")

		// Delete the user directly from Keycloak
		kc := getInternalKeycloakClient(t)
		err = kc.DeleteUser(ctx, realmName, userID)
		require.NoError(t, err, "Failed to delete user from Keycloak")
		t.Log("User deleted from Keycloak, waiting for reconciliation")

		// Trigger reconciliation by updating the CR
		updated := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser.Name,
			Namespace: kcUser.Namespace,
		}, updated)
		require.NoError(t, err)

		// Add an annotation to trigger reconciliation
		if updated.Annotations == nil {
			updated.Annotations = make(map[string]string)
		}
		updated.Annotations["test/reconcile-trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
		err = k8sClient.Update(ctx, updated)
		require.NoError(t, err)

		// Wait for the user to be recreated
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			// Check if user exists in Keycloak by searching for them
			users, err := kc.GetUsers(ctx, realmName, map[string]string{
				"username": userName,
			})
			if err != nil {
				return false, nil
			}
			return len(users) > 0, nil
		})
		require.NoError(t, err, "User was not recreated in Keycloak after deletion")
		t.Log("User was successfully reconciled (recreated) after manual deletion")
	})

	t.Run("ServiceAccountUser", func(t *testing.T) {
		// Create a confidential client with service accounts enabled
		clientName := fmt.Sprintf("sa-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Service Account Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true,
			"directAccessGrantsEnabled": false
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
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ClientUUID != "", nil
		})
		require.NoError(t, err, "Client did not become ready")
		t.Logf("Client %s is ready", clientName)

		// Create a KeycloakUser with clientRef to manage the service account user
		saUserName := fmt.Sprintf("sa-user-%d", time.Now().UnixNano())
		// Optional: provide a definition to customize the service account user
		saUserDef := rawJSON(`{
			"email": "service-account@example.com",
			"emailVerified": true,
			"attributes": {
				"department": ["backend-services"]
			}
		}`)
		saUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saUserName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
				Definition: &saUserDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, saUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, saUser)
		})

		// Wait for service account user to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      saUser.Name,
				Namespace: saUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Service account user did not become ready")

		// Verify status fields
		updatedUser := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      saUser.Name,
			Namespace: saUser.Namespace,
		}, updatedUser)
		require.NoError(t, err)
		require.True(t, updatedUser.Status.IsServiceAccount, "Status should indicate service account user")
		require.NotEmpty(t, updatedUser.Status.ClientID, "Status should contain client ID")
		require.NotEmpty(t, updatedUser.Status.UserID, "Status should contain user ID")
		require.Contains(t, updatedUser.Status.ResourcePath, "/users/", "ResourcePath should contain users path")
		t.Logf("Service account user %s is ready, userID=%s, clientID=%s, isServiceAccount=%v",
			saUserName, updatedUser.Status.UserID, updatedUser.Status.ClientID, updatedUser.Status.IsServiceAccount)
	})

	t.Run("ServiceAccountUserWithoutDefinition", func(t *testing.T) {
		// Create a confidential client with service accounts enabled
		clientName := fmt.Sprintf("sa-client-nodef-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Service Account Client NoDef",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": true
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
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ClientUUID != "", nil
		})
		require.NoError(t, err, "Client did not become ready")

		// Create a KeycloakUser with clientRef but NO definition (just read the service account)
		saUserName := fmt.Sprintf("sa-user-nodef-%d", time.Now().UnixNano())
		saUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saUserName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				ClientRef: &keycloakv1beta1.ResourceRef{Name: clientName},
				// No definition - just track the service account user
			},
		}
		require.NoError(t, k8sClient.Create(ctx, saUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, saUser)
		})

		// Wait for service account user to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      saUser.Name,
				Namespace: saUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Service account user without definition did not become ready")

		// Verify status
		updatedUser := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      saUser.Name,
			Namespace: saUser.Namespace,
		}, updatedUser)
		require.NoError(t, err)
		require.True(t, updatedUser.Status.IsServiceAccount)
		require.NotEmpty(t, updatedUser.Status.UserID)
		t.Logf("Service account user (no definition) is ready, userID=%s", updatedUser.Status.UserID)
	})

	t.Run("ServiceAccountUserInvalidClientRef", func(t *testing.T) {
		// Create a service account user referencing a non-existent client
		saUserName := fmt.Sprintf("sa-user-invalid-%d", time.Now().UnixNano())
		saUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saUserName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				ClientRef: &keycloakv1beta1.ResourceRef{Name: "non-existent-client"},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, saUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, saUser)
		})

		// Wait and verify the user is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakUser{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      saUserName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "User with invalid client ref should not be ready")
		require.Contains(t, updated.Status.Message, "KeycloakClient", "Error should mention KeycloakClient")
		t.Logf("Service account user correctly failed with invalid client ref, message: %s", updated.Status.Message)
	})

	t.Run("ServiceAccountUserClientWithoutServiceAccounts", func(t *testing.T) {
		// Create a client WITHOUT service accounts enabled
		clientName := fmt.Sprintf("no-sa-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "No Service Account Client",
			"enabled": true,
			"publicClient": false,
			"serviceAccountsEnabled": false
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
		require.NoError(t, err, "Client did not become ready")

		// Try to create a service account user for this client
		saUserName := fmt.Sprintf("sa-user-no-sa-%d", time.Now().UnixNano())
		saUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saUserName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				ClientRef: &keycloakv1beta1.ResourceRef{Name: clientName},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, saUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, saUser)
		})

		// Wait and verify the user is NOT ready (client doesn't have service accounts)
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakUser{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      saUserName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "User should not be ready when client has serviceAccountsEnabled=false")
		t.Logf("Service account user correctly failed for client without service accounts, message: %s", updated.Status.Message)
	})
}
