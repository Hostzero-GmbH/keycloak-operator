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

func TestKeycloakProtocolMapperE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "protocolmapper")

	t.Run("ClientProtocolMapper", func(t *testing.T) {
		// First create a client
		clientName := fmt.Sprintf("test-client-pm-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"enabled": true,
			"protocol": "openid-connect"
		}`, clientName))

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait for client to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ClientUUID != "", nil
		})
		require.NoError(t, err, "Client did not become ready")

		// Create protocol mapper
		mapperName := fmt.Sprintf("test-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"protocol": "openid-connect",
			"protocolMapper": "oidc-usermodel-attribute-mapper",
			"config": {
				"user.attribute": "department",
				"claim.name": "department",
				"jsonType.label": "String",
				"id.token.claim": "true",
				"access.token.claim": "true",
				"userinfo.token.claim": "true"
			}
		}`, mapperName))

		mapper := &keycloakv1beta1.KeycloakProtocolMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
				ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
				Definition: mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, mapper)
		})

		// Wait for mapper to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakProtocolMapper{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      mapper.Name,
				Namespace: mapper.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Protocol mapper did not become ready")
		t.Logf("Protocol mapper %s is ready", mapperName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakProtocolMapper{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      mapper.Name,
			Namespace: mapper.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.MapperID, "Mapper ID should be set")
		require.NotEmpty(t, updated.Status.MapperName, "Mapper name should be set")
		require.Equal(t, "client", updated.Status.ParentType, "Parent type should be client")
		require.NotEmpty(t, updated.Status.ParentID, "Parent ID should be set")
		t.Logf("Mapper ID: %s, Parent Type: %s", updated.Status.MapperID, updated.Status.ParentType)
	})

	t.Run("ClientScopeProtocolMapper", func(t *testing.T) {
		// First create a client scope
		scopeName := fmt.Sprintf("test-scope-pm-%d", time.Now().UnixNano())
		scopeDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"protocol": "openid-connect"
		}`, scopeName))

		scope := &keycloakv1beta1.KeycloakClientScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scopeName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientScopeSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: scopeDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, scope))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, scope)
		})

		// Wait for scope to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClientScope{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      scope.Name,
				Namespace: scope.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ResourcePath != "", nil
		})
		require.NoError(t, err, "Client scope did not become ready")

		// Create protocol mapper
		mapperName := fmt.Sprintf("scope-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"protocol": "openid-connect",
			"protocolMapper": "oidc-usermodel-attribute-mapper",
			"config": {
				"user.attribute": "phone",
				"claim.name": "phone_number",
				"jsonType.label": "String",
				"id.token.claim": "true",
				"access.token.claim": "true"
			}
		}`, mapperName))

		mapper := &keycloakv1beta1.KeycloakProtocolMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
				ClientScopeRef: &keycloakv1beta1.ResourceRef{Name: scopeName},
				Definition:     mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, mapper)
		})

		// Wait for mapper to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakProtocolMapper{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      mapper.Name,
				Namespace: mapper.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Client scope protocol mapper did not become ready")
		t.Logf("Client scope protocol mapper %s is ready", mapperName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakProtocolMapper{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      mapper.Name,
			Namespace: mapper.Namespace,
		}, updated))
		require.Equal(t, "clientScope", updated.Status.ParentType, "Parent type should be clientScope")
	})

	t.Run("ProtocolMapperCleanup", func(t *testing.T) {
		// First create a client
		clientName := fmt.Sprintf("cleanup-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(fmt.Sprintf(`{
			"clientId": "%s",
			"enabled": true,
			"protocol": "openid-connect"
		}`, clientName))

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcClient)
		})

		// Wait for client
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakClient{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcClient.Name,
				Namespace: kcClient.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Create mapper
		mapperName := fmt.Sprintf("cleanup-mapper-%d", time.Now().UnixNano())
		mapperDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"protocol": "openid-connect",
			"protocolMapper": "oidc-hardcoded-claim-mapper",
			"config": {
				"claim.name": "test-claim",
				"claim.value": "test-value",
				"id.token.claim": "true"
			}
		}`, mapperName))

		mapper := &keycloakv1beta1.KeycloakProtocolMapper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mapperName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
				ClientRef:  &keycloakv1beta1.ResourceRef{Name: clientName},
				Definition: mapperDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, mapper))

		// Wait for ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakProtocolMapper{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      mapper.Name,
				Namespace: mapper.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, mapper))

		// Verify deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakProtocolMapper{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      mapper.Name,
				Namespace: mapper.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Protocol mapper was not deleted")
		t.Logf("Protocol mapper %s cleanup verified", mapperName)
	})
}
