package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/controller"
)

// TestPreserveResourceAnnotation tests that the preserve-resource annotation prevents
// deletion of resources in Keycloak when the CR is deleted.
func TestPreserveResourceAnnotation(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, instanceNS := getOrCreateInstance(t)

	t.Run("PreserveRealmOnDeletion", func(t *testing.T) {
		// Create a realm with the preserve annotation
		realmName := fmt.Sprintf("preserve-realm-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					controller.PreserveResourceAnnotation: "true",
				},
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"displayName": "Preserved Realm",
					"enabled": true
				}`, realmName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))

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
		t.Logf("KeycloakRealm %s is ready with preserve annotation", realmName)

		// Verify realm exists in Keycloak
		kc := getInternalKeycloakClient(t)
		kcRealm, err := kc.GetRealm(ctx, realmName)
		require.NoError(t, err, "Realm should exist in Keycloak")
		require.NotNil(t, kcRealm)
		t.Logf("Verified realm %s exists in Keycloak", realmName)

		// Delete the CR
		require.NoError(t, k8sClient.Delete(ctx, realm))

		// Wait for CR to be deleted from K8s
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, &keycloakv1beta1.KeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "KeycloakRealm CR should be deleted from K8s")
		t.Logf("KeycloakRealm CR deleted from K8s")

		// Verify realm STILL exists in Keycloak (was preserved)
		kcRealm, err = kc.GetRealm(ctx, realmName)
		require.NoError(t, err, "Realm should still exist in Keycloak after CR deletion")
		require.NotNil(t, kcRealm)
		t.Logf("SUCCESS: Realm %s was preserved in Keycloak after CR deletion", realmName)

		// Cleanup: manually delete the realm from Keycloak
		t.Cleanup(func() {
			kc.DeleteRealm(ctx, realmName)
		})
	})

	t.Run("PreserveUserOnDeletion", func(t *testing.T) {
		// Create a realm first (without preserve annotation)
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-user")

		// Create a user with the preserve annotation
		userName := fmt.Sprintf("preserved-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"firstName": "Preserved",
			"lastName": "User",
			"enabled": true
		}`, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					controller.PreserveResourceAnnotation: "true",
				},
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))

		// Wait for user to be ready
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
		require.NoError(t, err, "KeycloakUser did not become ready")
		require.NotEmpty(t, userID, "User should have a UserID")
		t.Logf("KeycloakUser %s is ready with ID %s and preserve annotation", userName, userID)

		// Verify user exists in Keycloak
		kc := getInternalKeycloakClient(t)
		kcUserResp, err := kc.GetUser(ctx, realmName, userID)
		require.NoError(t, err, "User should exist in Keycloak")
		require.NotNil(t, kcUserResp)
		t.Logf("Verified user %s exists in Keycloak", userName)

		// Delete the CR
		require.NoError(t, k8sClient.Delete(ctx, kcUser))

		// Wait for CR to be deleted from K8s
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, &keycloakv1beta1.KeycloakUser{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "KeycloakUser CR should be deleted from K8s")
		t.Logf("KeycloakUser CR deleted from K8s")

		// Verify user STILL exists in Keycloak (was preserved)
		kcUserResp, err = kc.GetUser(ctx, realmName, userID)
		require.NoError(t, err, "User should still exist in Keycloak after CR deletion")
		require.NotNil(t, kcUserResp)
		t.Logf("SUCCESS: User %s was preserved in Keycloak after CR deletion", userName)

		// Cleanup: manually delete the user from Keycloak
		t.Cleanup(func() {
			kc.DeleteUser(ctx, realmName, userID)
		})
	})

	t.Run("PreserveClientOnDeletion", func(t *testing.T) {
		// Create a realm first
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-client")

		// Create a client with the preserve annotation
		clientName := fmt.Sprintf("preserved-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"name": "Preserved Client",
			"enabled": true,
			"publicClient": false
		}`, clientName))

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					controller.PreserveResourceAnnotation: "true",
				},
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))

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
		require.NoError(t, err, "KeycloakClient did not become ready")
		require.NotEmpty(t, clientUUID, "Client should have a UUID")
		t.Logf("KeycloakClient %s is ready with UUID %s and preserve annotation", clientName, clientUUID)

		// Verify client exists in Keycloak
		kc := getInternalKeycloakClient(t)
		clients, err := kc.GetClients(ctx, realmName, map[string]string{"clientId": clientName})
		require.NoError(t, err, "Should be able to query clients")
		require.Len(t, clients, 1, "Client should exist in Keycloak")
		t.Logf("Verified client %s exists in Keycloak", clientName)

		// Delete the CR
		require.NoError(t, k8sClient.Delete(ctx, kcClient))

		// Wait for CR to be deleted from K8s
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, &keycloakv1beta1.KeycloakClient{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "KeycloakClient CR should be deleted from K8s")
		t.Logf("KeycloakClient CR deleted from K8s")

		// Verify client STILL exists in Keycloak (was preserved)
		clients, err = kc.GetClients(ctx, realmName, map[string]string{"clientId": clientName})
		require.NoError(t, err, "Should be able to query clients")
		require.Len(t, clients, 1, "Client should still exist in Keycloak after CR deletion")
		t.Logf("SUCCESS: Client %s was preserved in Keycloak after CR deletion", clientName)

		// Cleanup: manually delete the client from Keycloak
		t.Cleanup(func() {
			kc.DeleteClient(ctx, realmName, clientUUID)
		})
	})

	t.Run("NormalDeletionWithoutAnnotation", func(t *testing.T) {
		// Create a realm WITHOUT the preserve annotation
		realmName := fmt.Sprintf("normal-delete-realm-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
				// No preserve annotation
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"displayName": "Normal Delete Realm",
					"enabled": true
				}`, realmName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))

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
		t.Logf("KeycloakRealm %s is ready (no preserve annotation)", realmName)

		// Verify realm exists in Keycloak
		kc := getInternalKeycloakClient(t)
		kcRealm, err := kc.GetRealm(ctx, realmName)
		require.NoError(t, err, "Realm should exist in Keycloak")
		require.NotNil(t, kcRealm)
		t.Logf("Verified realm %s exists in Keycloak", realmName)

		// Delete the CR
		require.NoError(t, k8sClient.Delete(ctx, realm))

		// Wait for CR to be deleted from K8s
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, &keycloakv1beta1.KeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "KeycloakRealm CR should be deleted from K8s")
		t.Logf("KeycloakRealm CR deleted from K8s")

		// Verify realm was ALSO deleted from Keycloak (normal behavior)
		_, err = kc.GetRealm(ctx, realmName)
		require.Error(t, err, "Realm should be deleted from Keycloak (normal deletion without preserve annotation)")
		t.Logf("SUCCESS: Realm %s was properly deleted from Keycloak (normal behavior)", realmName)
	})

	t.Run("PreserveAnnotationWithWrongValue", func(t *testing.T) {
		// Create a realm with preserve annotation set to something other than "true"
		realmName := fmt.Sprintf("wrong-value-realm-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					controller.PreserveResourceAnnotation: "false", // Should NOT preserve
				},
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
		t.Logf("KeycloakRealm %s is ready with annotation value 'false'", realmName)

		// Delete the CR
		require.NoError(t, k8sClient.Delete(ctx, realm))

		// Wait for CR to be deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, &keycloakv1beta1.KeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err)

		// Verify realm was DELETED from Keycloak (annotation value "false" should not preserve)
		kc := getInternalKeycloakClient(t)
		_, err = kc.GetRealm(ctx, realmName)
		require.Error(t, err, "Realm should be deleted from Keycloak when annotation is not 'true'")
		t.Logf("SUCCESS: Realm %s was properly deleted (annotation value 'false' does not preserve)", realmName)
	})
}
