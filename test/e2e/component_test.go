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

func TestKeycloakComponentE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "component")

	t.Run("RSAKeyProvider", func(t *testing.T) {
		componentName := fmt.Sprintf("rsa-key-%d", time.Now().UnixNano())
		componentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"],
				"algorithm": ["RS256"]
			}
		}`, componentName))

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: componentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, component)
		})

		// Wait for component to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "RSA key provider component did not become ready")
		t.Logf("RSA key provider component %s is ready", componentName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakComponent{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      component.Name,
			Namespace: component.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.ComponentID, "Component ID should be set")
		require.NotEmpty(t, updated.Status.ComponentName, "Component name should be set")
		require.Equal(t, "org.keycloak.keys.KeyProvider", updated.Status.ProviderType, "Provider type should match")
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Component ID: %s, Provider Type: %s", updated.Status.ComponentID, updated.Status.ProviderType)
	})

	t.Run("HMACKeyProvider", func(t *testing.T) {
		componentName := fmt.Sprintf("hmac-key-%d", time.Now().UnixNano())
		componentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "hmac-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"secretSize": ["64"],
				"algorithm": ["HS256"]
			}
		}`, componentName))

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: componentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, component)
		})

		// Wait for component to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "HMAC key provider component did not become ready")
		t.Logf("HMAC key provider component %s is ready", componentName)
	})

	t.Run("AESKeyProvider", func(t *testing.T) {
		componentName := fmt.Sprintf("aes-key-%d", time.Now().UnixNano())
		componentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "aes-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"secretSize": ["16"]
			}
		}`, componentName))

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: componentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, component)
		})

		// Wait for component to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "AES key provider component did not become ready")
		t.Logf("AES key provider component %s is ready", componentName)
	})

	t.Run("ComponentUpdate", func(t *testing.T) {
		componentName := fmt.Sprintf("update-component-%d", time.Now().UnixNano())
		componentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"]
			}
		}`, componentName))

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: componentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, component)
		})

		// Wait for component to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Update the component with different priority
		updated := &keycloakv1beta1.KeycloakComponent{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      component.Name,
			Namespace: component.Namespace,
		}, updated))

		updatedDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["200"],
				"keySize": ["2048"]
			}
		}`, componentName))
		updated.Spec.Definition = updatedDef
		require.NoError(t, k8sClient.Update(ctx, updated))

		// Wait for update to be processed
		time.Sleep(2 * time.Second)

		// Verify still ready
		final := &keycloakv1beta1.KeycloakComponent{}
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, final); err != nil {
				return false, nil
			}
			return final.Status.Ready, nil
		})
		require.NoError(t, err, "Component did not become ready after update")
		t.Logf("Component %s updated successfully", componentName)
	})

	t.Run("ComponentCleanup", func(t *testing.T) {
		componentName := fmt.Sprintf("cleanup-component-%d", time.Now().UnixNano())
		componentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"]
			}
		}`, componentName))

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: componentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, component))

		// Verify deleted from Kubernetes
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakComponent{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Component was not deleted")
		t.Logf("Component %s cleanup verified", componentName)
	})
}
