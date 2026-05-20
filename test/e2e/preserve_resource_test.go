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
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/controller"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
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

// preserveAnnotations returns the standard preserve-on-deletion annotation map
// used by every subtest in TestPreserveResourceAnnotationAcrossResources.
func preserveAnnotations() map[string]string {
	return map[string]string{
		controller.PreserveResourceAnnotation: "true",
	}
}

// deleteCRAndWait deletes the given CR and waits until it has been removed
// from Kubernetes (finalizer must have been released by the operator). It
// fails the test if the CR is still present after the timeout.
func deleteCRAndWait(t *testing.T, obj client.Object) {
	t.Helper()
	require.NoError(t, k8sClient.Delete(ctx, obj))

	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	probe := obj.DeepCopyObject().(client.Object)
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, key, probe)
		return errors.IsNotFound(err), nil
	})
	require.NoErrorf(t, err, "CR %T %s/%s was not removed from Kubernetes", obj, obj.GetNamespace(), obj.GetName())
}

// TestPreserveResourceAnnotationAcrossResources exercises the
// keycloak.hostzero.com/preserve-resource annotation on every CRD that maps
// to a Keycloak object (except realm/user/client which are covered by
// TestPreserveResourceAnnotation, and KeycloakIdentityProviderMapper which is
// covered by TestKeycloakIdentityProviderMapperE2E/PreserveAnnotation).
//
// For each kind we:
//  1. Create the CR with the preserve annotation
//  2. Wait for it to become Ready (and capture its Keycloak ID/alias)
//  3. Delete the CR and wait for it to disappear from Kubernetes
//  4. Verify the underlying object STILL exists in Keycloak
//  5. Clean up the Keycloak object so the test is idempotent
func TestPreserveResourceAnnotationAcrossResources(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	kc := getInternalKeycloakClient(t)

	t.Run("PreserveClusterKeycloakRealm", func(t *testing.T) {
		clusterInstanceName := getOrCreateClusterInstance(t)
		realmName := fmt.Sprintf("preserve-cluster-realm-%d", time.Now().UnixNano())

		clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:        realmName,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{Name: clusterInstanceName},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"enabled": true
				}`, realmName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterRealm))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm did not become ready")

		deleteCRAndWait(t, clusterRealm)

		_, err = kc.GetRealm(ctx, realmName)
		require.NoError(t, err, "ClusterKeycloakRealm should be preserved in Keycloak after CR deletion")
		t.Cleanup(func() { _ = kc.DeleteRealm(ctx, realmName) })
	})

	t.Run("PreserveClientScope", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-cs")
		scopeName := fmt.Sprintf("preserve-scope-%d", time.Now().UnixNano())

		scope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:        scopeName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"protocol": "openid-connect"
				}`, scopeName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, scope))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: scope.Name, Namespace: scope.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ResourcePath != "", nil
		})
		require.NoError(t, err, "KeycloakClientScope did not become ready")

		deleteCRAndWait(t, scope)

		preservedScope, err := kc.GetClientScopeByName(ctx, realmName, scopeName)
		require.NoError(t, err, "Client scope should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preservedScope, "Client scope should still exist in Keycloak")
		if preservedScope.ID != nil {
			scopeID := *preservedScope.ID
			t.Cleanup(func() { _ = kc.DeleteClientScope(ctx, realmName, scopeID) })
		}
	})

	t.Run("PreserveProtocolMapper", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-pm")

		// Parent client scope (no preserve annotation; will be deleted with the realm).
		scopeName := fmt.Sprintf("pm-parent-scope-%d", time.Now().UnixNano())
		scope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{Name: scopeName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"protocol": "openid-connect"
				}`, scopeName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, scope))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, scope) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: scope.Name, Namespace: scope.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "parent KeycloakClientScope did not become ready")

		// Protocol mapper attached to the scope, annotated for preservation.
		mapperName := fmt.Sprintf("preserve-pm-%d", time.Now().UnixNano())
		mapper := &keycloakv1beta1.KeycloakProtocolMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:        mapperName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
				ClientScopeRef: &keycloakv1beta1.ResourceRef{Name: scopeName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"protocol": "openid-connect",
					"protocolMapper": "oidc-usermodel-attribute-mapper",
					"config": {
						"user.attribute": "phone",
						"claim.name": "phone_number",
						"jsonType.label": "String",
						"id.token.claim": "true",
						"access.token.claim": "true"
					}
				}`, mapperName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))

		var parentID string
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakProtocolMapper{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: mapper.Name, Namespace: mapper.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			parentID = updated.Status.ParentID
			return parentID != "", nil
		})
		require.NoError(t, err, "KeycloakProtocolMapper did not become ready")

		deleteCRAndWait(t, mapper)

		preservedMapper, err := kc.GetClientScopeProtocolMapperByName(ctx, realmName, parentID, mapperName)
		require.NoError(t, err, "Protocol mapper should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preservedMapper, "Protocol mapper should still exist in Keycloak")
		if preservedMapper.ID != nil {
			mapperID := *preservedMapper.ID
			t.Cleanup(func() { _ = kc.DeleteClientScopeProtocolMapper(ctx, realmName, parentID, mapperID) })
		}
	})

	t.Run("PreserveGroup", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-grp")
		groupName := fmt.Sprintf("preserve-group-%d", time.Now().UnixNano())

		group := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        groupName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{"name": "%s"}`, groupName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, group))

		var groupID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: group.Name, Namespace: group.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			groupID = updated.Status.GroupID
			return groupID != "", nil
		})
		require.NoError(t, err, "KeycloakGroup did not become ready")

		deleteCRAndWait(t, group)

		preserved, err := kc.GetGroup(ctx, realmName, groupID)
		require.NoError(t, err, "Group should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Group should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteGroup(ctx, realmName, groupID) })
	})

	t.Run("PreserveRealmRole", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-role")
		roleName := fmt.Sprintf("preserve-role-%d", time.Now().UnixNano())

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:        roleName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"description": "preserve-role test"
				}`, roleName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, role))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.RoleName != "", nil
		})
		require.NoError(t, err, "KeycloakRole did not become ready")

		deleteCRAndWait(t, role)

		preserved, err := kc.GetRealmRole(ctx, realmName, roleName)
		require.NoError(t, err, "Realm role should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Realm role should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteRealmRole(ctx, realmName, roleName) })
	})

	t.Run("PreserveRoleMapping", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-rm")

		// Create the user that owns the mapping.
		userName := fmt.Sprintf("preserve-rm-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{"username": "%s", "enabled": true}`, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{Name: userName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, kcUser) })

		var userID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: kcUser.Name, Namespace: kcUser.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			userID = updated.Status.UserID
			return userID != "", nil
		})
		require.NoError(t, err, "KeycloakUser did not become ready")

		// Map the built-in offline_access realm role to the user, annotated to preserve.
		mappingName := fmt.Sprintf("preserve-rm-%d", time.Now().UnixNano())
		mapping := &keycloakv1beta1.KeycloakRoleMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:        mappingName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
				Subject: keycloakv1beta1.RoleMappingSubject{
					UserRef: &keycloakv1beta1.ResourceRef{Name: userName},
				},
				Role: &keycloakv1beta1.RoleDefinition{Name: "offline_access"},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapping))

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRoleMapping{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: mapping.Name, Namespace: mapping.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakRoleMapping did not become ready")

		deleteCRAndWait(t, mapping)

		// The mapping is "preserved" when the role assignment on the user is still present in Keycloak.
		roleMappings, err := kc.GetUserRealmRoleMappings(ctx, realmName, userID)
		require.NoError(t, err, "Should be able to fetch realm role mappings for the user")
		found := false
		for _, r := range roleMappings {
			if r.Name != nil && *r.Name == "offline_access" {
				found = true
				break
			}
		}
		require.True(t, found, "Role assignment should be preserved on the user after CR deletion")

		t.Cleanup(func() {
			role, gErr := kc.GetRealmRole(ctx, realmName, "offline_access")
			if gErr != nil || role == nil {
				return
			}
			_ = kc.DeleteRealmRolesFromUser(ctx, realmName, userID, []keycloak.RoleRepresentation{*role})
		})
	})

	t.Run("PreserveComponent", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-cmp")
		componentName := fmt.Sprintf("preserve-cmp-%d", time.Now().UnixNano())

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:        componentName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"providerId": "rsa-generated",
					"providerType": "org.keycloak.keys.KeyProvider",
					"config": {
						"priority": ["100"],
						"keySize": ["2048"],
						"algorithm": ["RS256"]
					}
				}`, componentName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))

		var componentID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: component.Name, Namespace: component.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			componentID = updated.Status.ComponentID
			return componentID != "", nil
		})
		require.NoError(t, err, "KeycloakComponent did not become ready")

		deleteCRAndWait(t, component)

		preserved, err := kc.GetComponent(ctx, realmName, componentID)
		require.NoError(t, err, "Component should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Component should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteComponent(ctx, realmName, componentID) })
	})

	t.Run("PreserveIdentityProvider", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-idp")
		idpName := fmt.Sprintf("preserve-idp-%d", time.Now().UnixNano())

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:        idpName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"alias": "%s",
					"providerId": "oidc",
					"enabled": true,
					"config": {
						"clientId": "preserve-client",
						"clientSecret": "preserve-secret",
						"authorizationUrl": "https://idp.example.com/auth",
						"tokenUrl": "https://idp.example.com/token",
						"defaultScope": "openid"
					}
				}`, idpName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakIdentityProvider did not become ready")

		deleteCRAndWait(t, idp)

		preserved, err := kc.GetIdentityProvider(ctx, realmName, idpName)
		require.NoError(t, err, "Identity provider should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Identity provider should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteIdentityProvider(ctx, realmName, idpName) })
	})

	t.Run("PreserveOrganization", func(t *testing.T) {
		// Organizations require Keycloak 26+.
		instance := &keycloakv1beta1.KeycloakInstance{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: instanceNS}, instance))
		if len(instance.Status.Version) < 2 || instance.Status.Version[0:2] < "26" {
			t.Skipf("Organizations require Keycloak 26+, current version: %q", instance.Status.Version)
		}

		realmName := createTestRealmWithOrganizations(t, instanceName, instanceNS, "preserve-org")
		orgName := fmt.Sprintf("preserve-org-%d", time.Now().UnixNano())

		org := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:        orgName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(fmt.Sprintf(`{
					"name": "%s",
					"alias": "%s",
					"enabled": true,
					"domains": [{"name": "%s.example.com", "verified": false}]
				}`, orgName, orgName, orgName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, org))

		var orgID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakOrganization{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: org.Name, Namespace: org.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			orgID = updated.Status.OrganizationID
			return orgID != "", nil
		})
		require.NoError(t, err, "KeycloakOrganization did not become ready")

		deleteCRAndWait(t, org)

		preserved, err := kc.GetOrganization(ctx, realmName, orgID)
		require.NoError(t, err, "Organization should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Organization should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteOrganization(ctx, realmName, orgID) })
	})

	t.Run("PreserveAuthenticationFlow", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-flow")
		flowAlias := fmt.Sprintf("preserve-flow-%d", time.Now().UnixNano())

		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:        flowAlias,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "preserve-resource e2e test flow",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(`[
					{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}
				]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))

		var flowID string
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, updated); err != nil {
				return false, nil
			}
			if !updated.Status.Ready {
				return false, nil
			}
			flowID = updated.Status.FlowID
			return flowID != "", nil
		})
		require.NoError(t, err, "KeycloakAuthenticationFlow did not become ready")

		deleteCRAndWait(t, flow)

		preserved, err := kc.GetAuthenticationFlowByAlias(ctx, realmName, flowAlias)
		require.NoError(t, err, "Authentication flow should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Authentication flow should still exist in Keycloak")
		t.Cleanup(func() { _ = kc.DeleteAuthenticationFlow(ctx, realmName, flowID) })
	})

	t.Run("PreserveRequiredAction", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, instanceNS, "preserve-ra")
		raName := fmt.Sprintf("preserve-ra-%d", time.Now().UnixNano())

		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:        raName,
				Namespace:   testNamespace,
				Annotations: preserveAnnotations(),
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(`{
					"alias": "VERIFY_EMAIL",
					"name": "Verify Email",
					"providerId": "VERIFY_EMAIL",
					"enabled": true,
					"defaultAction": false,
					"priority": 50
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: ra.Name, Namespace: ra.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.Alias != "", nil
		})
		require.NoError(t, err, "KeycloakRequiredAction did not become ready")

		deleteCRAndWait(t, ra)

		preserved, err := kc.GetRequiredAction(ctx, realmName, "VERIFY_EMAIL")
		require.NoError(t, err, "Required action should be preserved in Keycloak after CR deletion")
		require.NotNil(t, preserved, "Required action should still exist in Keycloak")
		// VERIFY_EMAIL is a built-in action; we can't truly "delete" it. The realm
		// gets garbage-collected via createTestRealm's t.Cleanup, which removes
		// all configuration including this required action.
	})
}
