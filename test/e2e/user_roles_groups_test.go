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

// TestKeycloakUserRolesGroupsE2E covers the typed spec.realmRoles / spec.groups
// fields on KeycloakUser and the one-home rejection of those keys inside
// spec.definition.
func TestKeycloakUserRolesGroupsE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "userroles")

	waitUserReady := func(t *testing.T, name string) *keycloakv1beta1.KeycloakUser {
		t.Helper()
		updated := &keycloakv1beta1.KeycloakUser{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser %s did not become ready (status: %s, message: %s)", name, updated.Status.Status, updated.Status.Message)
		return updated
	}

	t.Run("RealmRolesAndGroups", func(t *testing.T) {
		if !canConnectToKeycloak() {
			t.Skip("Skipping - cannot connect to Keycloak from test environment")
		}

		suffix := time.Now().UnixNano()

		// Create two realm roles and a group to assign.
		roleA := fmt.Sprintf("role-a-%d", suffix)
		roleB := fmt.Sprintf("role-b-%d", suffix)
		for _, roleName := range []string{roleA, roleB} {
			roleDef := rawJSON(`{"description": "e2e user role"}`)
			role := &keycloakv1beta1.KeycloakRole{
				ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: testNamespace},
				Spec: keycloakv1beta1.KeycloakRoleSpec{
					RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
					Name:       strPtr(roleName),
					Definition: roleDef,
				},
			}
			require.NoError(t, k8sClient.Create(ctx, role))
			t.Cleanup(func() { k8sClient.Delete(ctx, role) })
		}

		groupName := fmt.Sprintf("group-%d", suffix)
		group := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{Name: groupName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(groupName),
				Definition: rawJSON(`{}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, group))
		t.Cleanup(func() { k8sClient.Delete(ctx, group) })

		// Wait for the roles and the group to be ready.
		for _, name := range []string{roleA, roleB} {
			err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				updated := &keycloakv1beta1.KeycloakRole{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, updated); err != nil {
					return false, nil
				}
				return updated.Status.Ready, nil
			})
			require.NoError(t, err, "KeycloakRole %s did not become ready", name)
		}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: groupName, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakGroup did not become ready")

		// Create a user with both roles and the group membership.
		userName := fmt.Sprintf("roles-user-%d", suffix)
		userDef := rawJSON(`{"enabled": true}`)
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{Name: userName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Username:   strPtr(userName),
				Definition: &userDef,
				RealmRoles: &[]string{roleA, roleB},
				Groups:     &[]string{groupName},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() { k8sClient.Delete(ctx, kcUser) })

		updated := waitUserReady(t, userName)
		userID := updated.Status.UserID
		require.NotEmpty(t, userID)

		kc := getInternalKeycloakClient(t)

		hasRealmRoles := func(want map[string]bool) func(context.Context) (bool, error) {
			return func(ctx context.Context) (bool, error) {
				mappings, err := kc.GetUserRealmRoleMappings(ctx, realmName, userID)
				if err != nil {
					return false, nil
				}
				got := make(map[string]bool)
				for _, m := range mappings {
					if m.Name != nil {
						got[*m.Name] = true
					}
				}
				for name, wanted := range want {
					if got[name] != wanted {
						return false, nil
					}
				}
				return true, nil
			}
		}

		// Both roles assigned and the group joined.
		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true,
			hasRealmRoles(map[string]bool{roleA: true, roleB: true})), "realm roles were not assigned")

		groups, err := kc.GetUserGroups(ctx, realmName, userID)
		require.NoError(t, err)
		var groupNames []string
		for _, g := range groups {
			if g.Name != nil {
				groupNames = append(groupNames, *g.Name)
			}
		}
		require.Contains(t, groupNames, groupName, "user should be a member of the group")

		// Shrink the authoritative set: roleB and the group membership must be removed.
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, kcUser))
		kcUser.Spec.RealmRoles = &[]string{roleA}
		kcUser.Spec.Groups = &[]string{}
		require.NoError(t, k8sClient.Update(ctx, kcUser))

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true,
			hasRealmRoles(map[string]bool{roleA: true, roleB: false})), "roleB was not removed")

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			groups, err := kc.GetUserGroups(ctx, realmName, userID)
			if err != nil {
				return false, nil
			}
			return len(groups) == 0, nil
		}), "group membership was not removed")
	})

	// Regression for #124. spec.clientRoles drives the composite role-mappings
	// cleanup path, which no other subtest reaches. The user must reach Ready
	// rather than RoleGroupReconcileError, and dropping a client from the map
	// must actually remove its roles.
	t.Run("ClientRoles", func(t *testing.T) {
		if !canConnectToKeycloak() {
			t.Skip("Skipping - cannot connect to Keycloak from test environment")
		}

		suffix := time.Now().UnixNano()
		kc := getInternalKeycloakClient(t)

		// Two clients, each carrying a role, so one can later go stale.
		type testClient struct{ crName, clientID, role string }
		clients := []testClient{
			{fmt.Sprintf("cr-client-a-%d", suffix), fmt.Sprintf("client-a-%d", suffix), "role-a"},
			{fmt.Sprintf("cr-client-b-%d", suffix), fmt.Sprintf("client-b-%d", suffix), "role-b"},
		}
		for _, c := range clients {
			clientDef := rawJSON(`{"enabled": true, "serviceAccountsEnabled": true}`)
			client := &keycloakv1beta1.KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: c.crName, Namespace: testNamespace},
				Spec: keycloakv1beta1.KeycloakClientSpec{
					RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
					ClientId:   strPtr(c.clientID),
					Definition: &clientDef,
				},
			}
			require.NoError(t, k8sClient.Create(ctx, client))
			t.Cleanup(func() { k8sClient.Delete(ctx, client) })

			require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				updated := &keycloakv1beta1.KeycloakClient{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: c.crName, Namespace: testNamespace}, updated); err != nil {
					return false, nil
				}
				return updated.Status.Ready, nil
			}), "KeycloakClient %s did not become ready", c.crName)

			role := &keycloakv1beta1.KeycloakRole{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-role", c.crName), Namespace: testNamespace},
				Spec: keycloakv1beta1.KeycloakRoleSpec{
					RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
					ClientRef:  &keycloakv1beta1.ResourceRef{Name: c.crName},
					Name:       strPtr(c.role),
					Definition: rawJSON(`{"description": "e2e client role"}`),
				},
			}
			require.NoError(t, k8sClient.Create(ctx, role))
			t.Cleanup(func() { k8sClient.Delete(ctx, role) })

			require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				updated := &keycloakv1beta1.KeycloakRole{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-role", c.crName), Namespace: testNamespace}, updated); err != nil {
					return false, nil
				}
				return updated.Status.Ready, nil
			}), "KeycloakRole for %s did not become ready", c.crName)
		}

		userName := fmt.Sprintf("client-roles-user-%d", suffix)
		userDef := rawJSON(`{"enabled": true}`)
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{Name: userName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Username:   strPtr(userName),
				Definition: &userDef,
				ClientRoles: &map[string][]string{
					clients[0].clientID: {clients[0].role},
					clients[1].clientID: {clients[1].role},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() { k8sClient.Delete(ctx, kcUser) })

		// Before the fix this never became Ready: the stale-cleanup pass treated
		// every client mapping as orphaned and deleted it via a clientId-shaped
		// path, so each reconcile ended in RoleGroupReconcileError.
		updated := waitUserReady(t, userName)
		userID := updated.Status.UserID
		require.NotEmpty(t, userID)

		clientRoleNames := func(clientID string) func(context.Context) ([]string, error) {
			return func(ctx context.Context) ([]string, error) {
				rep, err := kc.GetClientByClientID(ctx, realmName, clientID)
				if err != nil {
					return nil, err
				}
				mappings, err := kc.GetUserClientRoleMappings(ctx, realmName, userID, *rep.ID)
				if err != nil {
					return nil, err
				}
				var names []string
				for _, m := range mappings {
					if m.Name != nil {
						names = append(names, *m.Name)
					}
				}
				return names, nil
			}
		}

		for _, c := range clients {
			require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				names, err := clientRoleNames(c.clientID)(ctx)
				if err != nil {
					return false, nil
				}
				return len(names) == 1 && names[0] == c.role, nil
			}), "client role %s was not assigned on %s", c.role, c.clientID)
		}

		// Drop the second client from the map: its roles must be cleaned up while
		// the first client keeps its own.
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, kcUser))
		kcUser.Spec.ClientRoles = &map[string][]string{
			clients[0].clientID: {clients[0].role},
		}
		require.NoError(t, k8sClient.Update(ctx, kcUser))

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			names, err := clientRoleNames(clients[1].clientID)(ctx)
			if err != nil {
				return false, nil
			}
			return len(names) == 0, nil
		}), "stale client roles on %s were not cleaned up", clients[1].clientID)

		names, err := clientRoleNames(clients[0].clientID)(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{clients[0].role}, names, "roles on the still-wanted client must survive cleanup")

		// The user stays Ready after the cleanup pass.
		final := &keycloakv1beta1.KeycloakUser{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, final))
		require.True(t, final.Status.Ready, "user should remain ready (status: %s, message: %s)", final.Status.Status, final.Status.Message)
	})

	// Role/group keys inside spec.definition violate the one-home invariant.
	t.Run("DefinitionRoleKeysRejected", func(t *testing.T) {
		userName := fmt.Sprintf("def-roles-user-%d", time.Now().UnixNano())
		userDef := rawJSON(`{"enabled": true, "realmRoles": ["offline_access"]}`)
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{Name: userName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Username:   strPtr(userName),
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() { k8sClient.Delete(ctx, kcUser) })

		updated := &keycloakv1beta1.KeycloakUser{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Status == "InvalidDefinition", nil
		})
		require.NoError(t, err, "user with realmRoles in definition was not rejected (status: %s, message: %s)", updated.Status.Status, updated.Status.Message)
		require.False(t, updated.Status.Ready)
		require.Contains(t, updated.Status.Message, "spec.realmRoles")
	})
}
