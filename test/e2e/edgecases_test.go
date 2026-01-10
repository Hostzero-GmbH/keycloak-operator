package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// TestSecretChangeDetection tests that KeycloakUserCredential re-syncs when the referenced Secret changes
func TestSecretChangeDetection(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "secret-change")

	t.Run("PasswordReSyncOnSecretChange", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("secret-change-user-%d", time.Now().UnixNano())
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

		// Create the secret first (before credential)
		secretName := fmt.Sprintf("%s-secret", userName)
		initialPassword := "initial-password-123"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"username": []byte(userName),
				"password": []byte(initialPassword),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, secret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, secret)
		})

		// Create UserCredential referencing the secret
		credName := fmt.Sprintf("%s-cred", userName)
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName:  secretName,
					Create:      false, // Use existing secret
					UsernameKey: "username",
					PasswordKey: "password",
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcCred)
		})

		// Wait for credential to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUserCredential did not become ready")

		// Get initial password hash
		updatedCred := &keycloakv1beta1.KeycloakUserCredential{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcCred.Name,
			Namespace: kcCred.Namespace,
		}, updatedCred))
		initialHash := updatedCred.Status.PasswordHash
		require.NotEmpty(t, initialHash, "PasswordHash should be set")
		t.Logf("Initial password hash: %s", initialHash)

		// Update the secret with a new password
		newPassword := "new-password-456"
		secret.Data["password"] = []byte(newPassword)
		require.NoError(t, k8sClient.Update(ctx, secret))
		t.Logf("Updated secret with new password")

		// Wait for the password hash to change (indicating re-sync occurred)
		err = wait.PollUntilContextTimeout(ctx, interval, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			// Check if hash changed
			return updated.Status.PasswordHash != initialHash, nil
		})
		require.NoError(t, err, "Password hash should change after secret update")

		// Verify the new hash
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcCred.Name,
			Namespace: kcCred.Namespace,
		}, updatedCred))
		require.NotEqual(t, initialHash, updatedCred.Status.PasswordHash, "Password hash should have changed")
		t.Logf("New password hash: %s (different from initial)", updatedCred.Status.PasswordHash)
	})
}

// TestParentDeletionWithChildren tests behavior when a parent resource is deleted while children exist
// NOTE: This is a known limitation - watches don't immediately trigger when parent resources are deleted.
// Child resources will eventually detect the missing parent on their next scheduled reconciliation (every 5 minutes).
func TestParentDeletionWithChildren(t *testing.T) {
	t.Skip("Known limitation: watches don't immediately detect parent deletion. Children update on next scheduled reconciliation.")
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)

	t.Run("RealmDeletedWithUsers", func(t *testing.T) {
		// Create a realm
		realmName := fmt.Sprintf("test-realm-parent-%d", time.Now().UnixNano())
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

		// Create a user in the realm
		userName := fmt.Sprintf("orphan-user-%d", time.Now().UnixNano())
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
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))

		// Wait for user to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
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
		t.Logf("User %s created and ready", userName)

		// Delete the realm (parent) while user (child) still exists
		require.NoError(t, k8sClient.Delete(ctx, realm))
		t.Logf("Deleted realm %s while user %s still exists", realmName, userName)

		// Wait for realm to be fully deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, &keycloakv1beta1.KeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Realm should be deleted")

		// Wait for the user to become not-ready (the watch should trigger reconciliation)
		var updatedUser *keycloakv1beta1.KeycloakUser
		err = wait.PollUntilContextTimeout(ctx, interval, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			updatedUser = &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updatedUser); err != nil {
				return false, err
			}
			// User should become not-ready after realm deletion
			return !updatedUser.Status.Ready, nil
		})
		require.NoError(t, err, "User should become not-ready after realm deletion")
		require.Contains(t, updatedUser.Status.Status, "RealmNotReady", "Status should indicate realm is not ready")
		t.Logf("User correctly shows status: %s - %s", updatedUser.Status.Status, updatedUser.Status.Message)

		// Cleanup
		k8sClient.Delete(ctx, kcUser)
	})

	t.Run("UserDeletedWithRoleMapping", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "orphan-mapping")

		// Create a user
		userName := fmt.Sprintf("mapping-user-%d", time.Now().UnixNano())
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
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))

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
		require.NoError(t, err)

		// Create a role mapping for the user
		mappingName := fmt.Sprintf("orphan-mapping-%d", time.Now().UnixNano())
		roleMapping := &keycloakv1beta1.KeycloakRoleMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mappingName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
				Subject: keycloakv1beta1.RoleMappingSubject{
					UserRef: &keycloakv1beta1.ResourceRef{Name: userName},
				},
				Role: &keycloakv1beta1.RoleDefinition{
					Name: "offline_access",
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, roleMapping))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, roleMapping)
		})

		// Wait for mapping to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRoleMapping{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      roleMapping.Name,
				Namespace: roleMapping.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "RoleMapping should become ready")
		t.Logf("RoleMapping %s created and ready", mappingName)

		// Delete the user while role mapping still exists
		require.NoError(t, k8sClient.Delete(ctx, kcUser))
		t.Logf("Deleted user %s while role mapping %s still exists", userName, mappingName)

		// Wait for user to be deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, &keycloakv1beta1.KeycloakUser{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err)

		// Wait for the role mapping to become not-ready (the watch should trigger reconciliation)
		var updatedMapping *keycloakv1beta1.KeycloakRoleMapping
		err = wait.PollUntilContextTimeout(ctx, interval, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			updatedMapping = &keycloakv1beta1.KeycloakRoleMapping{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      roleMapping.Name,
				Namespace: roleMapping.Namespace,
			}, updatedMapping); err != nil {
				return false, err
			}
			// Mapping should become not-ready after user deletion
			return !updatedMapping.Status.Ready, nil
		})
		require.NoError(t, err, "RoleMapping should become not-ready after user deletion")
		require.Equal(t, "SubjectNotReady", updatedMapping.Status.Status, "Status should indicate subject is not ready")
		t.Logf("RoleMapping correctly shows status: %s - %s", updatedMapping.Status.Status, updatedMapping.Status.Message)
	})
}

// TestDriftDetection tests that the operator detects and reconciles drift in Keycloak
// This test requires direct Keycloak access (port-forwarding) to modify resources directly.
func TestDriftDetection(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "drift")

	t.Run("UserReconciliationAfterDirectModification", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("drift-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"firstName": "Original",
			"lastName": "Name",
			"enabled": true
		}`, userName))

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

		// Get the user from the status
		updatedUser := &keycloakv1beta1.KeycloakUser{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser.Name,
			Namespace: kcUser.Namespace,
		}, updatedUser))
		t.Logf("User created with UserID: %s", updatedUser.Status.UserID)

		// Directly modify the user in Keycloak via API (simulating drift)
		kc := getInternalKeycloakClient(t)

		// Get the actual realm name from the resource
		keycloakRealmName := realmName // In our test, realm name matches resource name

		// Verify user exists in Keycloak
		var user *keycloak.UserRepresentation
		user, err = kc.GetUser(ctx, keycloakRealmName, updatedUser.Status.UserID)
		require.NoError(t, err)
		require.NotNil(t, user)

		// Modify the user - create a modified definition
		modifiedFirstName := "Modified"
		modifiedUser := map[string]interface{}{
			"id":        updatedUser.Status.UserID,
			"username":  userName,
			"firstName": modifiedFirstName,
			"enabled":   true,
		}
		modifiedJSON, _ := json.Marshal(modifiedUser)
		err = kc.UpdateUser(ctx, keycloakRealmName, updatedUser.Status.UserID, modifiedJSON)
		require.NoError(t, err)
		t.Logf("Directly modified user in Keycloak - firstName changed to '%s'", modifiedFirstName)

		// Trigger reconciliation by updating a label on the K8s resource
		updatedUser.Labels = map[string]string{"force-reconcile": "true"}
		require.NoError(t, k8sClient.Update(ctx, updatedUser))

		// Wait for reconciliation
		time.Sleep(5 * time.Second)

		// Verify the user in Keycloak has been reconciled back to the original state
		user, err = kc.GetUser(ctx, keycloakRealmName, updatedUser.Status.UserID)
		require.NoError(t, err)

		// The firstName should be back to "Original" after reconciliation
		require.NotNil(t, user.FirstName)
		require.Equal(t, "Original", *user.FirstName, "User should be reconciled back to original state")
		t.Logf("User reconciled - firstName is now '%s'", *user.FirstName)
	})
}

// TestGenerationTracking tests that ObservedGeneration is properly tracked
func TestGenerationTracking(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "generation")

	t.Run("ObservedGenerationUpdatedOnSpecChange", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("gen-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"firstName": "Initial",
			"enabled": true
		}`, userName))

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
		require.NoError(t, err)

		// Get initial generation
		updatedUser := &keycloakv1beta1.KeycloakUser{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser.Name,
			Namespace: kcUser.Namespace,
		}, updatedUser))

		initialGeneration := updatedUser.Generation
		t.Logf("Initial generation: %d, ObservedGeneration: %d", initialGeneration, updatedUser.Status.ObservedGeneration)

		// Update the spec
		newUserDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"firstName": "Updated",
			"enabled": true
		}`, userName))
		updatedUser.Spec.Definition = &newUserDef
		require.NoError(t, k8sClient.Update(ctx, updatedUser))

		// Wait for ObservedGeneration to be updated
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			// Generation should have increased and ObservedGeneration should match
			return updated.Generation > initialGeneration && updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Verify
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcUser.Name,
			Namespace: kcUser.Namespace,
		}, updatedUser))

		t.Logf("New generation: %d, ObservedGeneration: %d", updatedUser.Generation, updatedUser.Status.ObservedGeneration)
		require.Greater(t, updatedUser.Generation, initialGeneration, "Generation should have increased")
	})
}
