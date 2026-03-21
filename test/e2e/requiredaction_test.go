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

func TestKeycloakRequiredActionE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "reqaction")

	t.Run("ConfigureBuiltInAction", func(t *testing.T) {
		raName := fmt.Sprintf("verify-email-%d", time.Now().UnixNano())
		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      raName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(`{
					"alias": "VERIFY_EMAIL",
					"name": "Verify Email",
					"providerId": "VERIFY_EMAIL",
					"enabled": true,
					"defaultAction": true,
					"priority": 50
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, ra)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Required action did not become ready")

		updated := &keycloakv1beta1.KeycloakRequiredAction{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      ra.Name,
			Namespace: ra.Namespace,
		}, updated))
		require.Equal(t, "VERIFY_EMAIL", updated.Status.Alias)
		require.NotEmpty(t, updated.Status.ResourcePath)
		t.Logf("Required action %s is ready", raName)
	})

	t.Run("UpdateAction", func(t *testing.T) {
		raName := fmt.Sprintf("update-pwd-%d", time.Now().UnixNano())
		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      raName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(`{
					"alias": "UPDATE_PASSWORD",
					"name": "Update Password",
					"providerId": "UPDATE_PASSWORD",
					"enabled": true,
					"defaultAction": false,
					"priority": 30
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, ra)
		})

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Update: set as default action
		current := &keycloakv1beta1.KeycloakRequiredAction{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      ra.Name,
			Namespace: ra.Namespace,
		}, current))

		current.Spec.Definition = rawJSON(`{
			"alias": "UPDATE_PASSWORD",
			"name": "Update Password",
			"providerId": "UPDATE_PASSWORD",
			"enabled": true,
			"defaultAction": true,
			"priority": 10
		}`)
		require.NoError(t, k8sClient.Update(ctx, current))

		// Wait for re-reconciliation
		time.Sleep(2 * time.Second)
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Required action did not become ready after update")
		t.Logf("Required action %s updated successfully", raName)
	})

	t.Run("VerifyInKeycloak", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		raName := fmt.Sprintf("totp-%d", time.Now().UnixNano())
		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      raName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(`{
					"alias": "CONFIGURE_TOTP",
					"name": "Configure OTP",
					"providerId": "CONFIGURE_TOTP",
					"enabled": true,
					"defaultAction": true,
					"priority": 10
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, ra)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Verify in Keycloak
		kc := getInternalKeycloakClient(t)
		action, err := kc.GetRequiredAction(ctx, realmName, "CONFIGURE_TOTP")
		require.NoError(t, err, "Failed to get required action from Keycloak")
		require.NotNil(t, action)
		require.NotNil(t, action.Enabled)
		require.True(t, *action.Enabled, "Required action should be enabled")
		require.NotNil(t, action.DefaultAction)
		require.True(t, *action.DefaultAction, "Required action should be a default action")
		t.Logf("Required action CONFIGURE_TOTP verified in Keycloak: enabled=%v, default=%v", *action.Enabled, *action.DefaultAction)
	})

	t.Run("Cleanup", func(t *testing.T) {
		raName := fmt.Sprintf("cleanup-ra-%d", time.Now().UnixNano())
		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      raName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: rawJSON(`{
					"alias": "VERIFY_EMAIL",
					"name": "Verify Email",
					"providerId": "VERIFY_EMAIL",
					"enabled": true,
					"defaultAction": false
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRequiredAction{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		require.NoError(t, k8sClient.Delete(ctx, ra))

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakRequiredAction{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ra.Name,
				Namespace: ra.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Required action was not deleted from Kubernetes")
		t.Logf("Required action %s cleanup verified", raName)
	})

	t.Run("MissingRealm", func(t *testing.T) {
		raName := fmt.Sprintf("missing-realm-ra-%d", time.Now().UnixNano())
		ra := &keycloakv1beta1.KeycloakRequiredAction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      raName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: "nonexistent-realm"},
				Definition: rawJSON(`{
					"alias": "VERIFY_EMAIL",
					"name": "Verify Email",
					"providerId": "VERIFY_EMAIL",
					"enabled": true
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ra))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, ra)
		})

		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakRequiredAction{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      ra.Name,
			Namespace: ra.Namespace,
		}, updated))
		require.False(t, updated.Status.Ready, "Required action should not be ready with missing realm")
		t.Logf("Required action %s correctly not ready: %s", raName, updated.Status.Message)
	})
}
