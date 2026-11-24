package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TestKeycloakUserRequiredActionsE2E covers spec.initialRequiredActions and the
// rejection of requiredActions inside spec.definition (#169).
func TestKeycloakUserRequiredActionsE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "useractions")

	// An action queued at creation must not come back once the user has
	// completed it, even when a later change makes the operator update the user.
	t.Run("InitialActionsAreNotReapplied", func(t *testing.T) {
		if !canConnectToKeycloak() {
			t.Skip("Skipping - cannot connect to Keycloak from test environment")
		}

		userName := fmt.Sprintf("actions-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{"enabled": true, "email": "%s@example.com", "firstName": "Before", "lastName": "User"}`, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{Name: userName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:               &keycloakv1beta1.ResourceRef{Name: realmName},
				Username:               strPtr(userName),
				Definition:             &userDef,
				InitialRequiredActions: []string{"UPDATE_PASSWORD"},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() { k8sClient.Delete(ctx, kcUser) })

		updated := &keycloakv1beta1.KeycloakUser{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser did not become ready (status: %s, message: %s)", updated.Status.Status, updated.Status.Message)
		userID := updated.Status.UserID
		require.NotEmpty(t, userID)

		kc := getInternalKeycloakClient(t)

		readUser := func(t *testing.T) map[string]interface{} {
			t.Helper()
			raw, err := kc.GetUserRaw(ctx, realmName, userID)
			require.NoError(t, err)
			var rep map[string]interface{}
			require.NoError(t, json.Unmarshal(raw, &rep))
			return rep
		}

		rep := readUser(t)
		require.ElementsMatch(t, []interface{}{"UPDATE_PASSWORD"}, rep["requiredActions"], "the action should be queued at creation")

		// Stand in for the user completing the action: Keycloak then removes it
		// from the queue.
		rep["requiredActions"] = []string{}
		completed, err := json.Marshal(rep)
		require.NoError(t, err)
		require.NoError(t, kc.UpdateUser(ctx, realmName, userID, completed))
		require.Empty(t, readUser(t)["requiredActions"], "the action should be gone after completion")

		// Change the definition so that the operator has to update the user.
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: testNamespace}, kcUser))
		changedDef := rawJSON(fmt.Sprintf(`{"enabled": true, "email": "%s@example.com", "firstName": "After", "lastName": "User"}`, userName))
		kcUser.Spec.Definition = &changedDef
		require.NoError(t, k8sClient.Update(ctx, kcUser))

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			raw, err := kc.GetUserRaw(ctx, realmName, userID)
			if err != nil {
				return false, nil
			}
			var current struct {
				FirstName string `json:"firstName"`
			}
			if err := json.Unmarshal(raw, &current); err != nil {
				return false, nil
			}
			return current.FirstName == "After", nil
		}), "the definition change did not reach Keycloak")

		require.Empty(t, readUser(t)["requiredActions"], "the update must not queue the completed action again")
	})

	// requiredActions inside spec.definition would be re-applied on every
	// reconcile, so the key is rejected.
	t.Run("DefinitionRequiredActionsRejected", func(t *testing.T) {
		userName := fmt.Sprintf("def-actions-user-%d", time.Now().UnixNano())
		userDef := rawJSON(`{"enabled": true, "requiredActions": ["UPDATE_PASSWORD"]}`)
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
		require.NoError(t, err, "user with requiredActions in definition was not rejected (status: %s, message: %s)", updated.Status.Status, updated.Status.Message)
		require.False(t, updated.Status.Ready)
		require.Contains(t, updated.Status.Message, "spec.initialRequiredActions")
	})
}
