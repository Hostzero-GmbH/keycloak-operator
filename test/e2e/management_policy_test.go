package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/controller"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// TestManagementPolicyObserve covers managementPolicy Observe on realms: the
// operator reads the realm, reports readiness and drift, and never writes to
// Keycloak until the policy is switched to Manage.
func TestManagementPolicyObserve(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, _ := getOrCreateInstance(t)
	kc := getInternalKeycloakClient(t)

	t.Run("ObserveMissingRealm", func(t *testing.T) {
		realmName := fmt.Sprintf("observe-missing-%d", time.Now().UnixNano())
		realm := newObservedRealm(realmName, instanceName, `{"enabled": true}`)
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { deleteRealmCR(t, realm) })

		updated := &keycloakv1beta1.KeycloakRealm{}
		waitForCondition(t, realm.Name, realm.Namespace, updated, func() bool {
			return updated.Status.Status == controller.NotFoundReason
		}, "status NotFound")
		require.False(t, updated.Status.Ready)
		require.Nil(t, meta.FindStatusCondition(updated.Status.Conditions, controller.DriftedConditionType))

		_, err := kc.GetRealmRaw(ctx, realmName)
		require.True(t, keycloak.IsNotFound(err), "Observe must not create the realm, got %v", err)
	})

	t.Run("ObserveExistingRealm", func(t *testing.T) {
		realmName := createRealmInKeycloak(t, kc, "observe-existing", "Live")

		realm := newObservedRealm(realmName, instanceName, `{"enabled": true, "displayName": "Wanted"}`)
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { deleteRealmCR(t, realm) })

		// Ready, with the drift reported but not corrected.
		updated := &keycloakv1beta1.KeycloakRealm{}
		waitForCondition(t, realm.Name, realm.Namespace, updated, func() bool {
			return updated.Status.Ready && meta.IsStatusConditionTrue(updated.Status.Conditions, controller.DriftedConditionType)
		}, "Ready with Drifted=True")
		require.Equal(t, controller.ObservedReason, updated.Status.Status)
		require.Equal(t, "/admin/realms/"+realmName, updated.Status.ResourcePath)
		drifted := meta.FindStatusCondition(updated.Status.Conditions, controller.DriftedConditionType)
		require.Equal(t, controller.DefinitionMismatchReason, drifted.Reason)
		require.True(t, strings.HasSuffix(drifted.Message, ": displayName"), "message: %s", drifted.Message)
		require.Equal(t, "Live", liveDisplayName(t, kc, realmName), "Observe must not update the realm")

		// A Manage-mode child reconciles in the observed realm.
		clientID := fmt.Sprintf("observe-child-%d", time.Now().UnixNano())
		child := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{Name: clientID, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef: &keycloakv1beta1.ResourceRef{Name: realm.Name},
				ClientId: strPtr(clientID),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, child))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, child) })
		readyChild := &keycloakv1beta1.KeycloakClient{}
		waitForReady(t, child.Name, child.Namespace, readyChild, func() bool { return readyChild.Status.Ready })

		// Deleting the observed CR keeps the realm.
		deleteRealmCR(t, realm)
		_, err := kc.GetRealmRaw(ctx, realmName)
		require.NoError(t, err, "Observe must not delete the realm")
	})

	t.Run("SwitchToManageTakesOwnership", func(t *testing.T) {
		realmName := createRealmInKeycloak(t, kc, "observe-switch", "Live")

		realm := newObservedRealm(realmName, instanceName, `{"enabled": true, "displayName": "Wanted"}`)
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { deleteRealmCR(t, realm) })

		updated := &keycloakv1beta1.KeycloakRealm{}
		waitForReady(t, realm.Name, realm.Namespace, updated, func() bool { return updated.Status.Ready })

		updated.Spec.ManagementPolicy = keycloakv1beta1.ManagementPolicyManage
		require.NoError(t, k8sClient.Update(ctx, updated))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			return liveDisplayName(t, kc, realmName) == "Wanted", nil
		})
		require.NoError(t, err, "Manage must apply the definition")

		managed := &keycloakv1beta1.KeycloakRealm{}
		waitForCondition(t, realm.Name, realm.Namespace, managed, func() bool {
			return managed.Status.Status == "Ready" &&
				meta.FindStatusCondition(managed.Status.Conditions, controller.DriftedConditionType) == nil
		}, "Drifted condition removed in Manage mode")
	})

	t.Run("ObserveClusterRealm", func(t *testing.T) {
		realmName := createRealmInKeycloak(t, kc, "observe-cluster", "Live")

		realm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: realmName},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				InstanceRef:      &keycloakv1beta1.NamespacedRef{Name: instanceName, Namespace: testNamespace},
				RealmName:        strPtr(realmName),
				ManagementPolicy: keycloakv1beta1.ManagementPolicyObserve,
				Definition:       rawJSON(`{"displayName": "Live"}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))

		updated := &keycloakv1beta1.ClusterKeycloakRealm{}
		waitForCondition(t, realm.Name, "", updated, func() bool {
			return updated.Status.Ready && meta.IsStatusConditionFalse(updated.Status.Conditions, controller.DriftedConditionType)
		}, "Ready with Drifted=False")
		require.Equal(t, controller.ObservedReason, updated.Status.Status)

		require.NoError(t, k8sClient.Delete(ctx, realm))
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: realm.Name}, &keycloakv1beta1.ClusterKeycloakRealm{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm CR should be deleted")
		_, err = kc.GetRealmRaw(ctx, realmName)
		require.NoError(t, err, "Observe must not delete the realm")
	})
}

func newObservedRealm(realmName, instanceName, definition string) *keycloakv1beta1.KeycloakRealm {
	return &keycloakv1beta1.KeycloakRealm{
		ObjectMeta: metav1.ObjectMeta{Name: realmName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRealmSpec{
			InstanceRef:      &keycloakv1beta1.ResourceRef{Name: instanceName},
			RealmName:        strPtr(realmName),
			ManagementPolicy: keycloakv1beta1.ManagementPolicyObserve,
			Definition:       rawJSON(definition),
		},
	}
}

// createRealmInKeycloak creates a realm directly in Keycloak, as the owner
// cluster would, and removes it at the end of the test.
func createRealmInKeycloak(t *testing.T, kc *keycloak.Client, prefix, displayName string) string {
	realmName := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	definition := fmt.Sprintf(`{"realm": %q, "enabled": true, "displayName": %q}`, realmName, displayName)
	require.NoError(t, kc.CreateRealmFromDefinition(ctx, json.RawMessage(definition)))
	t.Cleanup(func() { _ = kc.DeleteRealm(ctx, realmName) })
	return realmName
}

// deleteRealmCR deletes the CR and waits until the finalizer has run.
func deleteRealmCR(t *testing.T, realm *keycloakv1beta1.KeycloakRealm) {
	if err := k8sClient.Delete(ctx, realm); err != nil && !errors.IsNotFound(err) {
		require.NoError(t, err)
	}
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: realm.Name, Namespace: realm.Namespace}, &keycloakv1beta1.KeycloakRealm{})
		return errors.IsNotFound(err), nil
	})
	require.NoError(t, err, "KeycloakRealm CR should be deleted")
}

func liveDisplayName(t *testing.T, kc *keycloak.Client, realmName string) string {
	raw, err := kc.GetRealmRaw(ctx, realmName)
	require.NoError(t, err)
	var live struct {
		DisplayName string `json:"displayName"`
	}
	require.NoError(t, json.Unmarshal(raw, &live))
	return live.DisplayName
}
