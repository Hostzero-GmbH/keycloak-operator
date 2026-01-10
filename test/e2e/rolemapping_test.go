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
)

func TestKeycloakRoleMappingE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "rolemapping")

	t.Run("MapRealmRoleToUser", func(t *testing.T) {
		// Create a user first
		userName := fmt.Sprintf("rolemapping-user-%d", time.Now().UnixNano())
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

		// Create role mapping using inline role definition
		// The "offline_access" role is a default realm role in Keycloak
		mappingName := fmt.Sprintf("offline-access-to-%s", userName)
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
					Name: "offline_access", // Built-in Keycloak realm role
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
		require.NoError(t, err, "KeycloakRoleMapping did not become ready")

		// Verify status
		updatedMapping := &keycloakv1beta1.KeycloakRoleMapping{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      roleMapping.Name,
			Namespace: roleMapping.Namespace,
		}, updatedMapping))
		require.Equal(t, "Ready", updatedMapping.Status.Status)
		require.Equal(t, "user", updatedMapping.Status.SubjectType)
		require.Equal(t, "realm", updatedMapping.Status.RoleType)
		require.Equal(t, "offline_access", updatedMapping.Status.RoleName)
		t.Logf("Role mapping %s is ready, subject: %s, role: %s", mappingName, updatedMapping.Status.SubjectType, updatedMapping.Status.RoleName)
	})

	t.Run("MapRoleToGroup", func(t *testing.T) {
		// Create a group first
		groupName := fmt.Sprintf("rolemapping-group-%d", time.Now().UnixNano())
		groupDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, groupName))

		kcGroup := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: groupDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcGroup))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcGroup)
		})

		// Wait for group to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcGroup.Name,
				Namespace: kcGroup.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakGroup did not become ready")

		// Create role mapping for group
		mappingName := fmt.Sprintf("uma-auth-to-%s", groupName)
		roleMapping := &keycloakv1beta1.KeycloakRoleMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mappingName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
				Subject: keycloakv1beta1.RoleMappingSubject{
					GroupRef: &keycloakv1beta1.ResourceRef{Name: groupName},
				},
				Role: &keycloakv1beta1.RoleDefinition{
					Name: "uma_authorization", // Built-in Keycloak realm role
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
		require.NoError(t, err, "KeycloakRoleMapping for group did not become ready")

		// Verify status
		updatedMapping := &keycloakv1beta1.KeycloakRoleMapping{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      roleMapping.Name,
			Namespace: roleMapping.Namespace,
		}, updatedMapping))
		require.Equal(t, "group", updatedMapping.Status.SubjectType)
		require.Equal(t, "realm", updatedMapping.Status.RoleType)
		t.Logf("Group role mapping %s is ready", mappingName)
	})

	t.Run("InvalidSubjectRef", func(t *testing.T) {
		// Create role mapping with non-existent user
		mappingName := fmt.Sprintf("invalid-mapping-%d", time.Now().UnixNano())
		roleMapping := &keycloakv1beta1.KeycloakRoleMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mappingName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
				Subject: keycloakv1beta1.RoleMappingSubject{
					UserRef: &keycloakv1beta1.ResourceRef{Name: "non-existent-user"},
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

		// Wait for mapping to show error
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRoleMapping{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      roleMapping.Name,
				Namespace: roleMapping.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			// Should fail because user doesn't exist
			return updated.Status.Status == "SubjectNotReady", nil
		})
		require.NoError(t, err, "RoleMapping should show SubjectNotReady status")

		updated := &keycloakv1beta1.KeycloakRoleMapping{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      roleMapping.Name,
			Namespace: roleMapping.Namespace,
		}, updated))
		require.False(t, updated.Status.Ready)
		t.Logf("Role mapping correctly failed with: %s", updated.Status.Message)
	})

	t.Run("InvalidRoleName", func(t *testing.T) {
		// Create a user first
		userName := fmt.Sprintf("invalid-role-user-%d", time.Now().UnixNano())
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
		require.NoError(t, err)

		// Create role mapping with non-existent role
		mappingName := fmt.Sprintf("invalid-role-mapping-%d", time.Now().UnixNano())
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
					Name: "non-existent-role-xyz",
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, roleMapping))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, roleMapping)
		})

		// Wait for mapping to show error
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRoleMapping{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      roleMapping.Name,
				Namespace: roleMapping.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			// Should fail because role doesn't exist
			return updated.Status.Status == "RoleNotFound", nil
		})
		require.NoError(t, err, "RoleMapping should show RoleNotFound status")

		updated := &keycloakv1beta1.KeycloakRoleMapping{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      roleMapping.Name,
			Namespace: roleMapping.Namespace,
		}, updated))
		require.False(t, updated.Status.Ready)
		t.Logf("Role mapping correctly failed with: %s", updated.Status.Message)
	})
}

func TestKeycloakRoleMappingCleanup(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "rolemapping-cleanup")

	t.Run("RoleMappingRemovalOnDelete", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("cleanup-mapping-user-%d", time.Now().UnixNano())
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
		require.NoError(t, err)

		// Create role mapping
		mappingName := fmt.Sprintf("cleanup-mapping-%d", time.Now().UnixNano())
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
		require.NoError(t, err)

		// Delete the mapping
		require.NoError(t, k8sClient.Delete(ctx, roleMapping))

		// Wait for mapping to be deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      roleMapping.Name,
				Namespace: roleMapping.Namespace,
			}, &keycloakv1beta1.KeycloakRoleMapping{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "RoleMapping should be deleted")
		t.Log("RoleMapping cleanup verified")
	})
}

func TestKeycloakClientRoleMapping(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "clientrolemapping")

	t.Run("MapClientRoleToUser", func(t *testing.T) {
		// Create a client with roles
		clientName := fmt.Sprintf("client-role-test-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"enabled": true,
			"protocol": "openid-connect",
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
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakClient did not become ready")

		// Create a user
		userName := fmt.Sprintf("client-role-user-%d", time.Now().UnixNano())
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
		require.NoError(t, err)

		// Map a client role using clientId (the client needs to have roles defined)
		// We'll map the built-in "uma_protection" client role from realm-management client
		mappingName := fmt.Sprintf("client-role-mapping-%d", time.Now().UnixNano())
		realmMgmtClientId := "realm-management"
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
					Name:     "view-users", // Built-in role in realm-management client
					ClientID: &realmMgmtClientId,
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
		require.NoError(t, err, "Client role mapping did not become ready")

		// Verify status
		updatedMapping := &keycloakv1beta1.KeycloakRoleMapping{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      roleMapping.Name,
			Namespace: roleMapping.Namespace,
		}, updatedMapping))
		require.Equal(t, "Ready", updatedMapping.Status.Status)
		require.Equal(t, "client", updatedMapping.Status.RoleType)
		require.Equal(t, "view-users", updatedMapping.Status.RoleName)
		t.Logf("Client role mapping %s is ready, role type: %s", mappingName, updatedMapping.Status.RoleType)
	})
}
