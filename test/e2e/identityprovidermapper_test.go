package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/controller"
)

func TestKeycloakIdentityProviderMapperE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "idpmapper")

	t.Run("RoleIdentityProviderMapper", func(t *testing.T) {
		idp := createOIDCIdentityProvider(t, realmName, "role-idp")

		mapperName := fmt.Sprintf("role-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "oidc-role-idp-mapper",
			"config": {
				"syncMode": "FORCE",
				"claim": "roles",
				"claim.value": "mdmsupport",
				"role": "offline_access"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, mapper) })

		updated := waitForMapperReady(t, mapper)
		require.NotEmpty(t, updated.Status.MapperID, "mapper ID should be populated")
		assert.Equal(t, mapperName, updated.Status.MapperName)
		assert.Equal(t, idp.Name, updated.Status.IdentityProviderAlias)
		expectedPath := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers/%s",
			realmName, idp.Name, updated.Status.MapperID)
		assert.Equal(t, expectedPath, updated.Status.ResourcePath)
		t.Logf("OIDC role mapper %s created at %s", mapperName, expectedPath)
	})

	t.Run("HardcodedAttributeMapper", func(t *testing.T) {
		idp := createOIDCIdentityProvider(t, realmName, "attr-idp")

		// Create two mappers under the same parent IdP to verify they coexist
		// and are matched by name (not by ID).
		firstName := fmt.Sprintf("source-attr-%d", time.Now().UnixNano())
		firstDef := rawJSON(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "source",
				"attribute.value": "oidc"
			}
		}`)

		first := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      firstName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(firstName),
				Definition:          firstDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, first))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, first) })

		secondName := fmt.Sprintf("provider-name-%d", time.Now().UnixNano())
		secondDef := rawJSON(fmt.Sprintf(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "provider",
				"attribute.value": "%s"
			}
		}`, idp.Name))

		second := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secondName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(secondName),
				Definition:          secondDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, second))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, second) })

		firstReady := waitForMapperReady(t, first)
		secondReady := waitForMapperReady(t, second)

		require.NotEmpty(t, firstReady.Status.MapperID)
		require.NotEmpty(t, secondReady.Status.MapperID)
		assert.NotEqual(t, firstReady.Status.MapperID, secondReady.Status.MapperID,
			"each mapper should be assigned a distinct Keycloak ID")
	})

	t.Run("UpdateMapperConfig", func(t *testing.T) {
		idp := createOIDCIdentityProvider(t, realmName, "update-idp")

		mapperName := fmt.Sprintf("update-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "oidc-user-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"claim": "department",
				"user.attribute": "department"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, mapper) })

		ready := waitForMapperReady(t, mapper)
		originalGen := ready.Status.ObservedGeneration
		mapperID := ready.Status.MapperID

		// Patch the spec.definition with a new sync mode
		updatedDef := rawJSON(`{
			"identityProviderMapper": "oidc-user-attribute-idp-mapper",
			"config": {
				"syncMode": "FORCE",
				"claim": "department",
				"user.attribute": "department"
			}
		}`)

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			latest := &keycloakv1beta1.KeycloakIdentityProviderMapper{}
			if err := k8sClient.Get(ctx, namespacedName(mapper), latest); err != nil {
				return false, err
			}
			latest.Spec.Definition = updatedDef
			err := k8sClient.Update(ctx, latest)
			return err == nil, nil
		})
		require.NoError(t, err, "failed to update mapper spec")

		// Wait for ObservedGeneration to advance and MapperID to remain stable
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			latest := &keycloakv1beta1.KeycloakIdentityProviderMapper{}
			if err := k8sClient.Get(ctx, namespacedName(mapper), latest); err != nil {
				return false, nil
			}
			return latest.Status.Ready &&
				latest.Status.ObservedGeneration > originalGen &&
				latest.Status.MapperID == mapperID, nil
		})
		require.NoError(t, err, "mapper did not reflect updated definition")
	})

	t.Run("MapperCleanup", func(t *testing.T) {
		idp := createOIDCIdentityProvider(t, realmName, "cleanup-idp")

		mapperName := fmt.Sprintf("cleanup-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "source",
				"attribute.value": "oidc"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		_ = waitForMapperReady(t, mapper)

		require.NoError(t, k8sClient.Delete(ctx, mapper))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, namespacedName(mapper), &keycloakv1beta1.KeycloakIdentityProviderMapper{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "mapper CR was not deleted")

		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			mappers, err := kc.GetIdentityProviderMappers(ctx, realmName, idp.Name)
			require.NoError(t, err)
			for _, m := range mappers {
				if m.Name != nil {
					assert.NotEqual(t, mapperName, *m.Name, "mapper should be removed from Keycloak")
				}
			}
		}
	})

	t.Run("PreserveAnnotation", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)
		idp := createOIDCIdentityProvider(t, realmName, "preserve-idp")

		mapperName := fmt.Sprintf("preserve-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "preserved",
				"attribute.value": "yes"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					controller.PreserveResourceAnnotation: "true",
				},
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		_ = waitForMapperReady(t, mapper)

		// Delete CR; mapper must remain in Keycloak
		require.NoError(t, k8sClient.Delete(ctx, mapper))
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, namespacedName(mapper), &keycloakv1beta1.KeycloakIdentityProviderMapper{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "mapper CR was not deleted")

		kc := getInternalKeycloakClient(t)
		found := false
		mappers, err := kc.GetIdentityProviderMappers(ctx, realmName, idp.Name)
		require.NoError(t, err)
		for _, m := range mappers {
			if m.Name != nil && *m.Name == mapperName {
				found = true
				if m.ID != nil {
					t.Cleanup(func() {
						_ = kc.DeleteIdentityProviderMapper(ctx, realmName, idp.Name, *m.ID)
					})
				}
				break
			}
		}
		assert.True(t, found, "preserved mapper should still exist in Keycloak after CR deletion")
	})

	t.Run("ParentIdPNotReady", func(t *testing.T) {
		// Reference a parent IdP CR that doesn't exist yet
		parentName := fmt.Sprintf("missing-idp-%d", time.Now().UnixNano())
		mapperName := fmt.Sprintf("waiting-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "wait",
				"attribute.value": "yes"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: parentName},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, mapper) })

		// Wait for the controller to mark the mapper not-ready with reason ParentNotReady
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			latest := &keycloakv1beta1.KeycloakIdentityProviderMapper{}
			if err := k8sClient.Get(ctx, namespacedName(mapper), latest); err != nil {
				return false, nil
			}
			return !latest.Status.Ready && latest.Status.Status == "ParentNotReady", nil
		})
		require.NoError(t, err, "mapper should report ParentNotReady before parent exists")

		// Now create the parent IdP and assert the mapper transitions to Ready
		idp := createOIDCIdentityProviderWithName(t, realmName, parentName)
		require.Equal(t, parentName, idp.Name)

		_ = waitForMapperReady(t, mapper)
	})

	t.Run("ClusterRealm", func(t *testing.T) {
		clusterInstanceName := getOrCreateClusterInstance(t)

		clusterRealmName := fmt.Sprintf("idpmapper-cluster-realm-%d", time.Now().UnixNano())
		clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRealmName,
			},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{Name: clusterInstanceName},
				RealmName:          strPtr(clusterRealmName),
				Definition: rawJSON(`{
					"enabled": true
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clusterRealm))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, clusterRealm) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.ClusterKeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRealmName}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "ClusterKeycloakRealm did not become ready")

		idpName := fmt.Sprintf("cluster-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"providerId": "oidc",
			"enabled": true,
			"config": {
				"clientId": "test",
				"clientSecret": "test",
				"authorizationUrl": "https://idp.example.com/auth",
				"tokenUrl": "https://idp.example.com/token"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: clusterRealmName},
				Alias:           strPtr(idpName),
				Definition:      idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, idp) })

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, namespacedName(idp), updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakIdentityProvider on cluster realm did not become ready")

		mapperName := fmt.Sprintf("cluster-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(`{
			"identityProviderMapper": "hardcoded-attribute-idp-mapper",
			"config": {
				"syncMode": "INHERIT",
				"attribute": "cluster",
				"attribute.value": "yes"
			}
		}`)

		mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
				IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
				Name:                strPtr(mapperName),
				Definition:          mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, mapper) })

		_ = waitForMapperReady(t, mapper)
	})
}

// createOIDCIdentityProvider creates an OIDC KeycloakIdentityProvider on the
// given namespaced realm and waits for it to become Ready. The returned
// object's Name is the alias used to address mappers under it.
func createOIDCIdentityProvider(t *testing.T, realmName, suffix string) *keycloakv1beta1.KeycloakIdentityProvider {
	t.Helper()
	name := fmt.Sprintf("idp-%s-%d", suffix, time.Now().UnixNano())
	return createOIDCIdentityProviderWithName(t, realmName, name)
}

// createOIDCIdentityProviderWithName is the same as createOIDCIdentityProvider
// but with an explicit IdP CR name (and alias).
func createOIDCIdentityProviderWithName(t *testing.T, realmName, name string) *keycloakv1beta1.KeycloakIdentityProvider {
	t.Helper()
	idpDef := rawJSON(`{
		"providerId": "oidc",
		"enabled": true,
		"config": {
			"clientId": "test-client",
			"clientSecret": "test-secret",
			"authorizationUrl": "https://idp.example.com/auth",
			"tokenUrl": "https://idp.example.com/token",
			"defaultScope": "openid"
		}
	}`)

	idp := &keycloakv1beta1.KeycloakIdentityProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
			Alias:      strPtr(name),
			Definition: idpDef,
		},
	}
	require.NoError(t, k8sClient.Create(ctx, idp))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, idp) })

	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakIdentityProvider{}
		if err := k8sClient.Get(ctx, namespacedName(idp), updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakIdentityProvider %s did not become ready", name)
	return idp
}

func waitForMapperReady(t *testing.T, mapper *keycloakv1beta1.KeycloakIdentityProviderMapper) *keycloakv1beta1.KeycloakIdentityProviderMapper {
	t.Helper()
	updated := &keycloakv1beta1.KeycloakIdentityProviderMapper{}
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, namespacedName(mapper), updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakIdentityProviderMapper %s did not become ready", mapper.Name)
	return updated
}

func namespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}
