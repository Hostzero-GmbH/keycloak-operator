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

func TestKeycloakClientScopeE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "clientscope")

	t.Run("BasicClientScope", func(t *testing.T) {
		scopeName := fmt.Sprintf("test-scope-%d", time.Now().UnixNano())
		scopeDef := rawJSON(`{
			"description": "Test client scope",
			"protocol": "openid-connect"
		}`)

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(scopeName),
				Definition: scopeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clientScope))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clientScope)
		})

		// Wait for client scope to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientScope.Name,
				Namespace: clientScope.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Client scope did not become ready")
		t.Logf("Client scope %s is ready", scopeName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakClientScope{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      clientScope.Name,
			Namespace: clientScope.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Client scope resource path: %s", updated.Status.ResourcePath)
	})

	t.Run("ProtocolMappersInDefinitionRejected", func(t *testing.T) {
		scopeName := fmt.Sprintf("scope-inline-mappers-%d", time.Now().UnixNano())
		scopeDef := rawJSON(`{
			"description": "Scope with inline protocol mappers",
			"protocol": "openid-connect",
			"protocolMappers": [
				{
					"name": "department",
					"protocol": "openid-connect",
					"protocolMapper": "oidc-usermodel-attribute-mapper",
					"config": {"claim.name": "department", "user.attribute": "department"}
				}
			]
		}`)

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(scopeName),
				Definition: scopeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clientScope))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clientScope)
		})

		// Keycloak silently discards mappers on the client scope PUT, so the
		// operator refuses the inline form rather than accepting edits it cannot apply.
		updated := &keycloakv1beta1.KeycloakClientScope{}
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientScope.Name,
				Namespace: clientScope.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Status == "UnsupportedDefinitionField", nil
		})
		require.NoError(t, err, "Client scope with inline mappers should be rejected")
		require.False(t, updated.Status.Ready, "Scope must not be ready")
		require.Contains(t, updated.Status.Message, "KeycloakProtocolMapper",
			"message should point at the replacement CRD")
	})

	// Regression for #123: a mapper change must actually reach Keycloak. The inline
	// definition form silently no-ops on update, so KeycloakProtocolMapper is the
	// only supported path and has to survive an edit.
	t.Run("ScopeProtocolMapperUpdates", func(t *testing.T) {
		scopeName := fmt.Sprintf("scope-mapper-update-%d", time.Now().UnixNano())
		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(scopeName),
				Definition: rawJSON(`{"protocol": "openid-connect"}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clientScope))
		t.Cleanup(func() { k8sClient.Delete(ctx, clientScope) })

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: scopeName, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		}), "Client scope did not become ready")

		mapperName := fmt.Sprintf("m2m-%d", time.Now().UnixNano())
		mapper := &keycloakv1beta1.KeycloakProtocolMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
				ClientScopeRef: &keycloakv1beta1.ResourceRef{Name: scopeName},
				Name:           strPtr(mapperName),
				Definition: rawJSON(`{
					"protocol": "openid-connect",
					"protocolMapper": "oidc-hardcoded-claim-mapper",
					"config": {
						"claim.name": "is_m2m",
						"claim.value": "true",
						"id.token.claim": "true",
						"access.token.claim": "true",
						"jsonType.label": "boolean"
					}
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() { k8sClient.Delete(ctx, mapper) })

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakProtocolMapper{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: mapperName, Namespace: testNamespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		}), "Protocol mapper did not become ready")

		// Flip id.token.claim to false — the exact edit that was silently dropped.
		current := &keycloakv1beta1.KeycloakProtocolMapper{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: mapperName, Namespace: testNamespace}, current))
		current.Spec.Definition = rawJSON(`{
			"protocol": "openid-connect",
			"protocolMapper": "oidc-hardcoded-claim-mapper",
			"config": {
				"claim.name": "is_m2m",
				"claim.value": "true",
				"id.token.claim": "false",
				"access.token.claim": "true",
				"jsonType.label": "boolean"
			}
		}`)
		require.NoError(t, k8sClient.Update(ctx, current))

		if !canConnectToKeycloak() {
			t.Skip("no direct Keycloak connectivity; skipping mapper value assertion")
		}

		kc := getInternalKeycloakClient(t)
		scopeID := ""
		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			scopes, err := kc.GetClientScopes(ctx, realmName)
			if err != nil {
				return false, nil
			}
			for i := range scopes {
				if scopes[i].Name != nil && *scopes[i].Name == scopeName {
					scopeID = *scopes[i].ID
					return true, nil
				}
			}
			return false, nil
		}), "client scope not found in Keycloak")

		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			found, err := kc.GetClientScopeProtocolMapperByName(ctx, realmName, scopeID, mapperName)
			if err != nil || found == nil {
				return false, nil
			}
			return found.Config != nil && found.Config["id.token.claim"] == "false", nil
		}), "mapper update never reached Keycloak")
		t.Logf("mapper %q update applied in Keycloak", mapperName)
	})

	t.Run("ClientScopeCleanup", func(t *testing.T) {
		scopeName := fmt.Sprintf("cleanup-scope-%d", time.Now().UnixNano())
		scopeDef := rawJSON(`{
			"protocol": "openid-connect"
		}`)

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Name:       strPtr(scopeName),
				Definition: scopeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clientScope))

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientScope.Name,
				Namespace: clientScope.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, clientScope))

		// Verify deleted from Kubernetes
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakClientScope{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientScope.Name,
				Namespace: clientScope.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Client scope was not deleted")
		t.Logf("Client scope %s cleanup verified", scopeName)
	})
}
