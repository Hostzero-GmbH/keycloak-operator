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

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakClientScopeE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "clientscope")

	t.Run("BasicClientScope", func(t *testing.T) {
		scopeName := fmt.Sprintf("test-scope-%d", time.Now().UnixNano())
		scopeDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"description": "Test client scope",
			"protocol": "openid-connect"
		}`, scopeName))

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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

	t.Run("ClientScopeWithProtocolMappers", func(t *testing.T) {
		scopeName := fmt.Sprintf("scope-with-mappers-%d", time.Now().UnixNano())
		scopeDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"description": "Scope with protocol mappers",
			"protocol": "openid-connect",
			"protocolMappers": [
				{
					"name": "department",
					"protocol": "openid-connect",
					"protocolMapper": "oidc-usermodel-attribute-mapper",
					"config": {
						"claim.name": "department",
						"user.attribute": "department",
						"jsonType.label": "String",
						"id.token.claim": "true",
						"access.token.claim": "true"
					}
				}
			]
		}`, scopeName))

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: scopeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, clientScope))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, clientScope)
		})

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
		require.NoError(t, err, "Client scope with mappers did not become ready")
		t.Logf("Client scope with protocol mappers %s is ready", scopeName)
	})

	t.Run("ClientScopeCleanup", func(t *testing.T) {
		scopeName := fmt.Sprintf("cleanup-scope-%d", time.Now().UnixNano())
		scopeDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"protocol": "openid-connect"
		}`, scopeName))

		clientScope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
