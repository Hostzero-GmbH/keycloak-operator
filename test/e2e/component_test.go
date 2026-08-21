package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestKeycloakComponentAdoptsExistingUserProfileComponentE2E(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "component-user-profile-adopt")
	kc := getInternalKeycloakClient(t)

	// Simulate saving the user profile through the Keycloak Admin UI/API before
	// the operator-owned KeycloakComponent exists. Keycloak persists that config
	// as a declarative-user-profile component, and in current Keycloak versions
	// that component may have no name.
	require.NoError(t, kc.Update(ctx, fmt.Sprintf("/admin/realms/%s/users/profile", realmName), map[string]interface{}{
		"attributes": []map[string]interface{}{
			{"name": "username"},
			{"name": "email"},
			{
				"name": "team",
				"permissions": map[string][]string{
					"view": {"admin", "user"},
					"edit": {"admin", "user"},
				},
			},
		},
	}))

	components, err := kc.GetComponents(ctx, realmName, map[string]string{
		"type": "org.keycloak.userprofile.UserProfileProvider",
	})
	require.NoError(t, err)
	require.Len(t, components, 1, "precondition: Keycloak should have created one persisted user-profile component")
	require.NotNil(t, components[0].ID)
	existingComponentID := *components[0].ID

	componentName := fmt.Sprintf("user-profile-component-%d", time.Now().UnixNano())
	component := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
			Name:     strPtr("declarative-user-profile"),
			Definition: rawJSON(`{
				"providerId": "declarative-user-profile",
				"providerType": "org.keycloak.userprofile.UserProfileProvider",
				"config": {
					"kc.user.profile.config": ["{\"attributes\":[{\"name\":\"username\"},{\"name\":\"email\"},{\"name\":\"department\",\"permissions\":{\"view\":[\"admin\",\"user\"],\"edit\":[\"admin\",\"user\"]}}]}"]
				}
			}`),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, component))
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, component)
	})

	updated := &keycloakv1beta1.KeycloakComponent{}
	err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      component.Name,
			Namespace: component.Namespace,
		}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "user-profile component did not become ready")

	components, err = kc.GetComponents(ctx, realmName, map[string]string{
		"type": "org.keycloak.userprofile.UserProfileProvider",
	})
	require.NoError(t, err)
	require.Len(t, components, 1, "operator should update/adopt the existing user-profile component instead of creating a duplicate")
	require.Equal(t, existingComponentID, updated.Status.ComponentID, "operator should adopt the Keycloak-created component, not replace it")
}

func TestKeycloakComponentE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "component")

	t.Run("RSAKeyProvider", func(t *testing.T) {
		componentName := fmt.Sprintf("rsa-key-%d", time.Now().UnixNano())
		componentDef := rawJSON(`{
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"],
				"algorithm": ["RS256"]
			}
		}`)

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(componentName),
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
		componentDef := rawJSON(`{
			"providerId": "hmac-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"secretSize": ["64"],
				"algorithm": ["HS256"]
			}
		}`)

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(componentName),
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
		componentDef := rawJSON(`{
			"providerId": "aes-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"secretSize": ["16"]
			}
		}`)

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(componentName),
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
		componentDef := rawJSON(`{
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"]
			}
		}`)

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(componentName),
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

		updatedDef := rawJSON(`{
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["200"],
				"keySize": ["2048"]
			}
		}`)
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
		componentDef := rawJSON(`{
			"providerId": "rsa-generated",
			"providerType": "org.keycloak.keys.KeyProvider",
			"config": {
				"priority": ["100"],
				"keySize": ["2048"]
			}
		}`)

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(componentName),
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

	t.Run("ConfigSecretRef", func(t *testing.T) {
		componentName := fmt.Sprintf("ldap-secret-%d", time.Now().UnixNano())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName + "-creds",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"bindCredential": "ldap-bind-password",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, secret))
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, secret)
		})

		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:        &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:            strPtr(componentName),
				ConfigSecretRef: &keycloakv1beta1.ConfigSecretRef{Name: secret.Name},
				Definition: rawJSON(`{
					"providerId": "ldap",
					"providerType": "org.keycloak.storage.UserStorageProvider",
					"config": {
						"enabled": ["true"],
						"vendor": ["other"],
						"connectionUrl": ["ldap://ldap.example.com:389"],
						"bindDn": ["cn=admin,dc=example,dc=com"],
						"usersDn": ["ou=users,dc=example,dc=com"],
						"usernameLDAPAttribute": ["uid"],
						"rdnLDAPAttribute": ["uid"],
						"uuidLDAPAttribute": ["entryUUID"],
						"userObjectClasses": ["inetOrgPerson, organizationalPerson"],
						"editMode": ["READ_ONLY"]
					}
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, component)
		})

		updated := &keycloakv1beta1.KeycloakComponent{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "LDAP component with configSecretRef did not become ready: %s", updated.Status.Message)

		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			raw, err := kc.GetComponentRaw(ctx, realmName, updated.Status.ComponentID)
			require.NoError(t, err)
			var parsed struct {
				Config map[string][]string `json:"config"`
			}
			require.NoError(t, json.Unmarshal(raw, &parsed))
			require.NotEmpty(t, parsed.Config["bindCredential"], "bindCredential should be present after secret merge")
		}

		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret))
		secret.Data = map[string][]byte{"bindCredential": []byte("rotated-password")}
		require.NoError(t, k8sClient.Update(ctx, secret))
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakComponent{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, check); err != nil {
				return false, nil
			}
			return check.Status.Ready, nil
		})
		require.NoError(t, err, "component should stay Ready after secret update")
	})

	t.Run("ConfigSecretRefMissingSecret", func(t *testing.T) {
		componentName := fmt.Sprintf("ldap-missing-secret-%d", time.Now().UnixNano())
		component := &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakComponentSpec{
				RealmRef:        &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:            strPtr(componentName),
				ConfigSecretRef: &keycloakv1beta1.ConfigSecretRef{Name: componentName + "-missing"},
				Definition: rawJSON(`{
					"providerId": "ldap",
					"providerType": "org.keycloak.storage.UserStorageProvider",
					"config": {
						"enabled": ["true"]
					}
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, component))
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, component)
		})

		updated := &keycloakv1beta1.KeycloakComponent{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      component.Name,
				Namespace: component.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Status == "ConfigSecretError", nil
		})
		require.NoError(t, err, "missing config secret should set ConfigSecretError, got %q: %s", updated.Status.Status, updated.Status.Message)
		require.False(t, updated.Status.Ready)
	})
}
