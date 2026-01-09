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

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakRoleE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "role")

	t.Run("RealmRole", func(t *testing.T) {
		roleName := fmt.Sprintf("test-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"description": "Test realm role"
		}`, roleName))

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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

		// Now create a client role
		roleName := fmt.Sprintf("client-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"description": "Test client role"
		}`, roleName))

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
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
		roleDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"description": "Role with attributes",
			"attributes": {
				"permission": ["read", "write"]
			}
		}`, roleName))

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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

	t.Run("RoleCleanup", func(t *testing.T) {
		roleName := fmt.Sprintf("cleanup-role-%d", time.Now().UnixNano())
		roleDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, roleName))

		role := &keycloakv1beta1.KeycloakRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRoleSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
