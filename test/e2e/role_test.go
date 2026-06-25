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

func TestKeycloakRoleE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "role")

	t.Run("RealmRole", func(t *testing.T) {
		roleName := fmt.Sprintf("test-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(`{
			"description": "Test realm role"
		}`)

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(roleName),
				Definition: roleDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, role))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, role)
		})

		// Wait for role to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      role.Name,
				Namespace: role.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Realm role did not become ready")
		t.Logf("Realm role %s is ready", roleName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakRole{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      role.Name,
			Namespace: role.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.RoleName, "Role name should be set")
		require.False(t, updated.Status.IsClientRole, "Should not be a client role")
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Role name: %s, Resource path: %s", updated.Status.RoleName, updated.Status.ResourcePath)
	})

	t.Run("ClientRole", func(t *testing.T) {
		// First create a client
		clientName := fmt.Sprintf("test-client-for-role-%d", time.Now().UnixNano())
		clientDef := rawJSON(`{
			"enabled": true
		}`)

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				ClientId:   strPtr(clientName),
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

		// Now create a client role
		roleName := fmt.Sprintf("client-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(`{
			"description": "Test client role"
		}`)

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
				Name:       strPtr(roleName),
				Definition: roleDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, role))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, role)
		})

		// Wait for role to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      role.Name,
				Namespace: role.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Client role did not become ready")
		t.Logf("Client role %s is ready", roleName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakRole{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      role.Name,
			Namespace: role.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.RoleName, "Role name should be set")
		require.True(t, updated.Status.IsClientRole, "Should be a client role")
		require.NotEmpty(t, updated.Status.ClientID, "Client ID should be set")
		t.Logf("Client role name: %s, Client ID: %s", updated.Status.RoleName, updated.Status.ClientID)
	})

	t.Run("RoleWithAttributes", func(t *testing.T) {
		roleName := fmt.Sprintf("role-attrs-%d", time.Now().UnixNano())
		roleDef := rawJSON(`{
			"description": "Role with attributes",
			"attributes": {
				"permission": ["read", "write"]
			}
		}`)

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(roleName),
				Definition: roleDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, role))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, role)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      role.Name,
				Namespace: role.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Role with attributes did not become ready")
		t.Logf("Role with attributes %s is ready", roleName)
	})

	t.Run("CompositeRealmRole", func(t *testing.T) {
		// Regression test: composite: true plus a composites block must end up
		// as an actual composite role in Keycloak, and the membership must be
		// reconciled on update.
		clientName := fmt.Sprintf("composite-target-%d", time.Now().UnixNano())
		clientDef := rawJSON(`{
			"enabled": true
		}`)
		targetClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				ClientId:   strPtr(clientName),
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, targetClient))
		t.Cleanup(func() { k8sClient.Delete(ctx, targetClient) })
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: targetClient.Name, Namespace: targetClient.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ClientUUID != "", nil
		})
		require.NoError(t, err, "Target client did not become ready")

		clientRoleNames := []string{
			fmt.Sprintf("view-%d", time.Now().UnixNano()),
			fmt.Sprintf("edit-%d", time.Now().UnixNano()+1),
		}
		for _, rn := range clientRoleNames {
			rn := rn
			cr := &keycloakv1beta1.KeycloakRole{
				ObjectMeta: metav1.ObjectMeta{Name: rn, Namespace: testNamespace},
				Spec: keycloakv1beta1.KeycloakRoleSpec{
					RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
					ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
					Name:       strPtr(rn),
					Definition: rawJSON(`{}`),
				},
			}
			require.NoError(t, k8sClient.Create(ctx, cr))
			t.Cleanup(func() { k8sClient.Delete(ctx, cr) })
			err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				updated := &keycloakv1beta1.KeycloakRole{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, updated); err != nil {
					return false, nil
				}
				return updated.Status.Ready, nil
			})
			require.NoError(t, err, "Member client role %s did not become ready", rn)
		}

		baseRealmRoleName := fmt.Sprintf("base-realm-%d", time.Now().UnixNano())
		baseRealmRole := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{Name: baseRealmRoleName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(baseRealmRoleName),
				Definition: rawJSON(`{}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, baseRealmRole))
		t.Cleanup(func() { k8sClient.Delete(ctx, baseRealmRole) })
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: baseRealmRole.Name, Namespace: baseRealmRole.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Base realm role did not become ready")

		compositeName := fmt.Sprintf("composite-%d", time.Now().UnixNano())
		compositeDef := rawJSON(fmt.Sprintf(`{
			"description": "Composite role under test",
			"composite": true,
			"composites": {
				"realm": ["%s"],
				"client": {
					"%s": ["%s", "%s"]
				}
			}
		}`, baseRealmRoleName, clientName, clientRoleNames[0], clientRoleNames[1]))

		composite := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{Name: compositeName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(compositeName),
				Definition: compositeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, composite))
		t.Cleanup(func() { k8sClient.Delete(ctx, composite) })

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: composite.Name, Namespace: composite.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Composite role did not become ready")

		skipIfNoKeycloakAccess(t)
		kc := getInternalKeycloakClient(t)

		role, err := kc.GetRealmRole(ctx, realmName, compositeName)
		require.NoError(t, err, "Failed to get composite role from Keycloak")
		require.NotNil(t, role.Composite, "composite flag should be set on the role")
		require.True(t, *role.Composite, "composite flag should be true after sync")

		members, err := kc.GetRealmRoleComposites(ctx, realmName, compositeName)
		require.NoError(t, err, "Failed to list composites from Keycloak")
		require.Len(t, members, 3, "expected 3 composite members (1 realm + 2 client roles)")

		seen := map[string]bool{}
		for _, m := range members {
			if m.Name != nil {
				seen[*m.Name] = true
			}
		}
		require.True(t, seen[baseRealmRoleName], "base realm role should be a member")
		require.True(t, seen[clientRoleNames[0]], "first client role should be a member")
		require.True(t, seen[clientRoleNames[1]], "second client role should be a member")

		// Drop one member and verify the operator removes it from Keycloak too.
		compositeDef = rawJSON(fmt.Sprintf(`{
			"description": "Composite role under test",
			"composite": true,
			"composites": {
				"realm": ["%s"],
				"client": {
					"%s": ["%s"]
				}
			}
		}`, baseRealmRoleName, clientName, clientRoleNames[0]))

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: composite.Name, Namespace: composite.Namespace}, updated); err != nil {
				return false, err
			}
			updated.Spec.Definition = compositeDef
			if err := k8sClient.Update(ctx, updated); err != nil {
				if errors.IsConflict(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		})
		require.NoError(t, err, "Failed to update composite role definition")

		require.Eventually(t, func() bool {
			members, err := kc.GetRealmRoleComposites(ctx, realmName, compositeName)
			if err != nil {
				return false
			}
			return len(members) == 2
		}, timeout, interval, "expected composite member set to shrink to 2 after update")
	})

	t.Run("RoleCleanup", func(t *testing.T) {
		roleName := fmt.Sprintf("cleanup-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(`{}`)

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(roleName),
				Definition: roleDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, role))

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      role.Name,
				Namespace: role.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, role))

		// Verify deleted from Kubernetes
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakRole{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      role.Name,
				Namespace: role.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Role was not deleted")
		t.Logf("Role %s cleanup verified", roleName)
	})
}
