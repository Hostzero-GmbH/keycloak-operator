package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestKeycloakRealmE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)

	t.Run("BasicRealm", func(t *testing.T) {
		// Create realm with unique name to avoid conflicts
		realmName := fmt.Sprintf("test-realm-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				Definition: rawJSON(`{
					"displayName": "Test Realm",
					"enabled": true
				}`),
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

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: "non-existent-instance"},
				RealmName:   strPtr(realmName),
				Definition: rawJSON(`{
					"enabled": true
				}`),
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

	t.Run("MissingRealmNameRejected", func(t *testing.T) {
		realmName := fmt.Sprintf("realm-missing-name-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				// No spec.realmName: it is required, so the apiserver must reject this.
				Definition: rawJSON(`{"enabled": true}`),
			},
		}
		err := k8sClient.Create(ctx, realm)
		require.Error(t, err, "creating a realm without spec.realmName must be rejected")
		if err == nil {
			t.Cleanup(func() { k8sClient.Delete(ctx, realm) })
		}
		t.Logf("Realm without realmName correctly rejected: %v", err)
	})

	t.Run("SmtpSecretRef", func(t *testing.T) {
		realmName := fmt.Sprintf("smtp-secret-realm-%d", time.Now().UnixNano())

		// Create the SMTP credentials secret
		smtpSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName + "-smtp",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"user":     "smtp-user@example.com",
				"password": "super-secret-password",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, smtpSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, smtpSecret)
		})

		// Create realm with smtpSecretRef
		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				SmtpSecretRef: &keycloakv1beta1.SmtpSecretRefSpec{
					Name: realmName + "-smtp",
				},
				Definition: rawJSON(`{
					"enabled": true,
					"smtpServer": {
						"host": "smtp.example.com",
						"port": "587",
						"starttls": "true",
						"auth": "true"
					}
				}`),
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
		require.NoError(t, err, "KeycloakRealm with smtpSecretRef did not become ready")
		t.Logf("KeycloakRealm %s with smtpSecretRef is ready", realmName)

		// Verify SMTP credentials were injected by reading realm from Keycloak
		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			realmRaw, err := kc.GetRealmRaw(ctx, realmName)
			require.NoError(t, err, "Failed to get realm from Keycloak")

			var realmData map[string]interface{}
			require.NoError(t, json.Unmarshal(realmRaw, &realmData))

			smtp, ok := realmData["smtpServer"].(map[string]interface{})
			require.True(t, ok, "smtpServer should exist in realm")
			require.Equal(t, "smtp-user@example.com", smtp["user"], "SMTP user should be injected from secret")
			// Keycloak masks the password in API responses, so we just verify it's present
			require.NotEmpty(t, smtp["password"], "SMTP password should be set")
			require.Equal(t, "smtp.example.com", smtp["host"], "Existing SMTP fields should be preserved")
			t.Log("Verified: SMTP credentials from secret were injected into Keycloak realm")
		}
	})

	t.Run("SmtpSecretRefCustomKeys", func(t *testing.T) {
		realmName := fmt.Sprintf("smtp-custom-keys-%d", time.Now().UnixNano())

		// Create secret with custom key names
		smtpSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName + "-smtp",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"smtp-username": "custom-user@example.com",
				"smtp-password": "custom-secret",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, smtpSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, smtpSecret)
		})

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				SmtpSecretRef: &keycloakv1beta1.SmtpSecretRefSpec{
					Name:        realmName + "-smtp",
					UserKey:     "smtp-username",
					PasswordKey: "smtp-password",
				},
				Definition: rawJSON(`{
					"enabled": true,
					"smtpServer": {
						"host": "smtp.example.com",
						"port": "465"
					}
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

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
		require.NoError(t, err, "KeycloakRealm with custom smtpSecretRef keys did not become ready")
		t.Logf("KeycloakRealm %s with custom SMTP secret keys is ready", realmName)

		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			realmRaw, err := kc.GetRealmRaw(ctx, realmName)
			require.NoError(t, err)

			var realmData map[string]interface{}
			require.NoError(t, json.Unmarshal(realmRaw, &realmData))

			smtp, ok := realmData["smtpServer"].(map[string]interface{})
			require.True(t, ok, "smtpServer should exist")
			require.Equal(t, "custom-user@example.com", smtp["user"])
			// Keycloak masks the password in API responses
			require.NotEmpty(t, smtp["password"], "SMTP password should be set")
			t.Log("Verified: Custom SMTP secret keys work correctly")
		}
	})

	t.Run("SmtpSecretRefMissingSecret", func(t *testing.T) {
		realmName := fmt.Sprintf("smtp-missing-secret-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				SmtpSecretRef: &keycloakv1beta1.SmtpSecretRefSpec{
					Name: "nonexistent-smtp-secret",
				},
				Definition: rawJSON(`{
					"enabled": true
				}`),
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
		require.False(t, updated.Status.Ready, "Realm with missing SMTP secret should not be ready")
		require.Equal(t, "SmtpSecretError", updated.Status.Status)
		t.Logf("Realm correctly failed with missing SMTP secret, message: %s", updated.Status.Message)
	})

	t.Run("SmtpSecretRefMissingKey", func(t *testing.T) {
		realmName := fmt.Sprintf("smtp-missing-key-%d", time.Now().UnixNano())

		// Create secret with wrong keys
		smtpSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName + "-smtp",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"wrong-key": "some-value",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, smtpSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, smtpSecret)
		})

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				SmtpSecretRef: &keycloakv1beta1.SmtpSecretRefSpec{
					Name: realmName + "-smtp",
				},
				Definition: rawJSON(`{
					"enabled": true
				}`),
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
		require.False(t, updated.Status.Ready, "Realm with missing SMTP key should not be ready")
		require.Equal(t, "SmtpSecretError", updated.Status.Status)
		require.Contains(t, updated.Status.Message, "not found in SMTP secret")
		t.Logf("Realm correctly failed with missing key, message: %s", updated.Status.Message)
	})

	t.Run("RealmWithClusterInstanceRef", func(t *testing.T) {
		clusterInstanceName := getOrCreateClusterInstance(t)
		realmName := fmt.Sprintf("realm-cluster-ref-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{
					Name: clusterInstanceName,
				},
				RealmName: strPtr(realmName),
				Definition: rawJSON(`{
					"displayName": "Cluster Instance Realm",
					"enabled": true
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

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
		require.NoError(t, err, "KeycloakRealm with clusterInstanceRef did not become ready")

		updated := &keycloakv1beta1.KeycloakRealm{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      realm.Name,
			Namespace: realm.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.ResourcePath)
		require.NotNil(t, updated.Status.Instance)
		require.Equal(t, clusterInstanceName, updated.Status.Instance.ClusterInstanceRef,
			"Status should reference the cluster instance")
		t.Logf("KeycloakRealm %s with clusterInstanceRef is ready", realmName)
	})

	t.Run("RealmWithInvalidClusterInstanceRef", func(t *testing.T) {
		realmName := fmt.Sprintf("realm-bad-cluster-ref-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{
					Name: "nonexistent-cluster-instance",
				},
				RealmName: strPtr(realmName),
				Definition: rawJSON(`{
					"enabled": true
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, realm)
		})

		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakRealm{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      realmName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready,
			"Realm with nonexistent clusterInstanceRef should not be ready")
		require.Contains(t, updated.Status.Message, "ClusterKeycloakInstance")
		t.Logf("Realm correctly failed with invalid clusterInstanceRef, message: %s", updated.Status.Message)
	})

	t.Run("RealmWithNoInstanceRef", func(t *testing.T) {
		realmName := fmt.Sprintf("realm-no-ref-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				RealmName: strPtr(realmName),
				Definition: rawJSON(`{
					"enabled": true
				}`),
			},
		}
		err := k8sClient.Create(ctx, realm)
		require.Error(t, err, "realm with neither instanceRef nor clusterInstanceRef must be rejected")
		if err == nil {
			t.Cleanup(func() { k8sClient.Delete(ctx, realm) })
		}
		require.Contains(t, err.Error(), "exactly one of instanceRef or clusterInstanceRef must be set")
		t.Logf("Realm without instance ref correctly rejected: %v", err)
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
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmName),
				Definition: rawJSON(`{
					"enabled": true
				}`),
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

// TestSameNamespaceRefEnforcement verifies that a namespace set on instanceRef or
// realmRef is pruned by the CRD schema and the operator resolves in the CR's own namespace.
func TestSameNamespaceRefEnforcement(t *testing.T) {
	skipIfNoCluster(t)
	instanceName, _ := getOrCreateInstance(t)

	t.Run("InstanceRefNamespacePruned", func(t *testing.T) {
		realmName := fmt.Sprintf("same-ns-inst-%d", time.Now().UnixNano())
		obj := newUnstructured("KeycloakRealm", realmName, map[string]interface{}{
			"instanceRef": map[string]interface{}{
				"name":      instanceName,
				"namespace": "non-existent-namespace",
			},
			"realmName": realmName,
			"definition": map[string]interface{}{
				"enabled": true,
			},
		})

		require.NoError(t, k8sClient.Create(ctx, obj))
		t.Cleanup(func() { k8sClient.Delete(ctx, obj) })

		requireRefNamespacePruned(t, "KeycloakRealm", realmName, "instanceRef")

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			current := &keycloakv1beta1.KeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realmName, Namespace: testNamespace}, current); err != nil {
				return false, nil
			}
			return current.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakRealm should resolve its instance in its own namespace and become ready")
	})

	t.Run("RealmRefNamespacePruned", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, "same-ns-realmref")

		clientName := fmt.Sprintf("same-ns-client-%d", time.Now().UnixNano())
		obj := newUnstructured("KeycloakClient", clientName, map[string]interface{}{
			"realmRef": map[string]interface{}{
				"name":      realmName,
				"namespace": "non-existent-namespace",
			},
			"clientId": clientName,
			"definition": map[string]interface{}{
				"enabled":      true,
				"publicClient": true,
			},
		})

		require.NoError(t, k8sClient.Create(ctx, obj))
		t.Cleanup(func() { k8sClient.Delete(ctx, obj) })

		requireRefNamespacePruned(t, "KeycloakClient", clientName, "realmRef")

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			current := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: clientName, Namespace: testNamespace}, current); err != nil {
				return false, nil
			}
			return current.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakClient should resolve its realm in its own namespace and become ready")
	})
}

// newUnstructured builds a CR in testNamespace, used to submit a ref namespace
// that the Go ResourceRef type no longer allows.
func newUnstructured(kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keycloak.hostzero.com/v1beta1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": spec,
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "keycloak.hostzero.com", Version: "v1beta1", Kind: kind})
	return obj
}

// requireRefNamespacePruned asserts the named spec ref has no namespace field after a round-trip.
func requireRefNamespacePruned(t *testing.T, kind, name, refField string) {
	t.Helper()
	stored := &unstructured.Unstructured{}
	stored.SetGroupVersionKind(schema.GroupVersionKind{Group: "keycloak.hostzero.com", Version: "v1beta1", Kind: kind})
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, stored))
	ref, _, _ := unstructured.NestedMap(stored.Object, "spec", refField)
	_, hasNamespace := ref["namespace"]
	require.False(t, hasNamespace, "spec.%s.namespace should be pruned by the CRD schema", refField)
}
