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

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakRealmE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)

	t.Run("BasicRealm", func(t *testing.T) {
		// Create realm with unique name to avoid conflicts
		realmName := fmt.Sprintf("test-realm-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"displayName": "Test Realm",
					"enabled": true
				}`, realmName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

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
		t.Logf("KeycloakRealm %s is ready", realmName)
	})

	t.Run("InvalidInstanceRef", func(t *testing.T) {
		realmName := fmt.Sprintf("realm-invalid-ref-%d", time.Now().UnixNano())
		nonExistentNS := "non-existent-ns"

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: "non-existent-instance", Namespace: &nonExistentNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"enabled": true
				}`, realmName)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

		// Wait and verify the realm is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakRealm{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      realmName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Realm with invalid instance ref should not be ready")
		t.Logf("Realm correctly failed with invalid instance ref, message: %s", updated.Status.Message)
	})

	t.Run("InvalidRealmDefinition", func(t *testing.T) {
		realmName := fmt.Sprintf("realm-invalid-def-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				// Valid JSON but with conflicting/problematic realm config
				Definition: rawJSON(`{"realm": "", "enabled": true}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

		// Wait and verify the realm is NOT ready (empty realm name should fail)
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakRealm{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      realmName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Realm with empty name should not be ready")
		t.Logf("Realm correctly failed with invalid definition, message: %s", updated.Status.Message)
	})

	t.Run("ReconcileAfterManualDeletion", func(t *testing.T) {
		// Skip if not running in-cluster or without port-forward
		if !canConnectToKeycloak() {
			t.Skip("Skipping reconcile test - cannot connect to Keycloak from test environment")
		}

		// Create a realm
		realmName := fmt.Sprintf("realm-reconcile-%d", time.Now().UnixNano())
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
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

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
		t.Log("Realm is ready, now deleting it directly from Keycloak")

		// Delete the realm directly from Keycloak
		kc := getInternalKeycloakClient(t)
		err = kc.DeleteRealm(ctx, realmName)
		require.NoError(t, err, "Failed to delete realm from Keycloak")
		t.Log("Realm deleted from Keycloak, waiting for reconciliation")

		// Trigger reconciliation by updating the CR
		updated := &keycloakv1beta1.KeycloakRealm{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      realm.Name,
			Namespace: realm.Namespace,
		}, updated)
		require.NoError(t, err)

		// Add an annotation to trigger reconciliation
		if updated.Annotations == nil {
			updated.Annotations = make(map[string]string)
		}
		updated.Annotations["test/reconcile-trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
		err = k8sClient.Update(ctx, updated)
		require.NoError(t, err)

		// Wait for the realm to be recreated and ready again
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			// Check if realm exists in Keycloak
			_, err := kc.GetRealm(ctx, realmName)
			return err == nil, nil
		})
		require.NoError(t, err, "Realm was not recreated in Keycloak after deletion")
		t.Log("Realm was successfully reconciled (recreated) after manual deletion")
	})
}
