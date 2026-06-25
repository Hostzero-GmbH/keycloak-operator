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

func TestKeycloakIdentityProviderE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "idp")

	t.Run("OIDCIdentityProvider", func(t *testing.T) {
		idpName := fmt.Sprintf("test-oidc-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"displayName": "Test OIDC Provider",
			"providerId": "oidc",
			"enabled": true,
			"trustEmail": false,
			"storeToken": false,
			"addReadTokenRoleOnCreate": false,
			"firstBrokerLoginFlowAlias": "first broker login",
			"config": {
				"clientId": "test-client",
				"clientSecret": "test-secret",
				"authorizationUrl": "https://idp.example.com/auth",
				"tokenUrl": "https://idp.example.com/token",
				"userInfoUrl": "https://idp.example.com/userinfo",
				"defaultScope": "openid profile email"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, idp)
		})

		// Wait for IdP to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      idp.Name,
				Namespace: idp.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Identity provider did not become ready")
		t.Logf("OIDC identity provider %s is ready", idpName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakIdentityProvider{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      idp.Name,
			Namespace: idp.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Identity provider resource path: %s", updated.Status.ResourcePath)
	})

	t.Run("SAMLIdentityProvider", func(t *testing.T) {
		idpName := fmt.Sprintf("test-saml-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"displayName": "Test SAML Provider",
			"providerId": "saml",
			"enabled": true,
			"trustEmail": false,
			"storeToken": false,
			"addReadTokenRoleOnCreate": false,
			"firstBrokerLoginFlowAlias": "first broker login",
			"config": {
				"singleSignOnServiceUrl": "https://idp.example.com/sso",
				"nameIDPolicyFormat": "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
				"signatureAlgorithm": "RSA_SHA256"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, idp)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      idp.Name,
				Namespace: idp.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "SAML identity provider did not become ready")
		t.Logf("SAML identity provider %s is ready", idpName)
	})

	t.Run("GitHubIdentityProvider", func(t *testing.T) {
		idpName := fmt.Sprintf("test-github-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"displayName": "GitHub",
			"providerId": "github",
			"enabled": true,
			"trustEmail": true,
			"config": {
				"clientId": "github-client-id",
				"clientSecret": "github-client-secret",
				"defaultScope": "read:user user:email"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, idp)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      idp.Name,
				Namespace: idp.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "GitHub identity provider did not become ready")
		t.Logf("GitHub identity provider %s is ready", idpName)
	})

	t.Run("TokenExchangePermission", func(t *testing.T) {
		// Two real clients allowed to perform token-exchange with this IdP.
		// They must be Ready before the IdP can resolve their UUIDs.
		clientNames := []string{
			fmt.Sprintf("te-allowed-a-%d", time.Now().UnixNano()),
			fmt.Sprintf("te-allowed-b-%d", time.Now().UnixNano()),
		}
		for _, cn := range clientNames {
			clientDef := rawJSON(`{
				"enabled": true,
				"protocol": "openid-connect",
				"publicClient": false,
				"serviceAccountsEnabled": true
			}`)
			kcClient := &keycloakv1beta1.KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: cn, Namespace: testNamespace},
				Spec: keycloakv1beta1.KeycloakClientSpec{
					RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
					ClientId:   strPtr(cn),
					Definition: &clientDef,
				},
			}
			require.NoError(t, k8sClient.Create(ctx, kcClient))
			t.Cleanup(func() { _ = k8sClient.Delete(ctx, kcClient) })

			require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				got := &keycloakv1beta1.KeycloakClient{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cn, Namespace: testNamespace}, got); err != nil {
					return false, nil
				}
				return got.Status.Ready && got.Status.ClientUUID != "", nil
			}), "client %s did not become ready", cn)
		}

		// IdP with spec.tokenExchange.allowedClients targeting the two clients above.
		idpName := fmt.Sprintf("te-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"displayName": "TE Test IdP",
			"providerId": "oidc",
			"enabled": true,
			"firstBrokerLoginFlowAlias": "first broker login",
			"config": {
				"clientId": "te-dummy",
				"clientSecret": "te-dummy",
				"authorizationUrl": "https://te.example.com/auth",
				"tokenUrl": "https://te.example.com/token",
				"hideOnLoginPage": "true"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: idpName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
				TokenExchange: &keycloakv1beta1.IDPTokenExchangeSpec{
					AllowedClients: clientNames,
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, idp) })

		// Wait for both: parent Ready AND TE wired.
		var updated keycloakv1beta1.KeycloakIdentityProvider
		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, &updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready &&
				updated.Status.TokenExchange != nil &&
				updated.Status.TokenExchange.Enabled, nil
		}), "TE permission did not become enabled in time")

		require.NotEmpty(t, updated.Status.TokenExchange.PermissionID, "PermissionID should be populated")
		require.NotEmpty(t, updated.Status.TokenExchange.PolicyID, "PolicyID should be populated")
		require.Equal(t,
			fmt.Sprintf("hostzero-idp-%s-token-exchange", idpName),
			updated.Status.TokenExchange.PolicyName,
			"PolicyName follows the expected convention")
		require.Empty(t, updated.Status.TokenExchange.Message,
			"Message should be empty in the happy path")
		t.Logf("TE permission wired: policy=%s permission=%s",
			updated.Status.TokenExchange.PolicyID, updated.Status.TokenExchange.PermissionID)
	})

	t.Run("TokenExchangeWaitsForReferencedClient", func(t *testing.T) {
		// spec.tokenExchange points at a clientId that does not exist in Keycloak.
		// The IdP itself must still reach Ready=true (the soft-skip path), and the
		// status surface should communicate the wait without flipping Ready off.
		missing := fmt.Sprintf("te-missing-client-%d", time.Now().UnixNano())
		idpName := fmt.Sprintf("te-waiting-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"providerId": "oidc",
			"enabled": true,
			"firstBrokerLoginFlowAlias": "first broker login",
			"config": {
				"clientId": "te-dummy",
				"clientSecret": "te-dummy",
				"authorizationUrl": "https://te.example.com/auth",
				"tokenUrl": "https://te.example.com/token",
				"hideOnLoginPage": "true"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: idpName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
				TokenExchange: &keycloakv1beta1.IDPTokenExchangeSpec{
					AllowedClients: []string{missing},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, idp) })

		var updated keycloakv1beta1.KeycloakIdentityProvider
		require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, &updated); err != nil {
				return false, nil
			}
			// Parent IdP must be Ready even though the TE side is waiting.
			return updated.Status.Ready &&
				updated.Status.TokenExchange != nil &&
				updated.Status.TokenExchange.Message != "", nil
		}), "IdP did not reach Ready with waiting TE message in time")

		require.False(t, updated.Status.TokenExchange.Enabled,
			"TE should not report enabled while waiting on the referenced client")
		require.Contains(t, updated.Status.TokenExchange.Message, missing,
			"waiting message should name the missing client (%s)", missing)
		t.Logf("TE soft-wait surfaced: %s", updated.Status.TokenExchange.Message)
	})

	t.Run("IdentityProviderCleanup", func(t *testing.T) {
		idpName := fmt.Sprintf("cleanup-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(`{
			"providerId": "oidc",
			"enabled": true,
			"config": {
				"clientId": "test",
				"clientSecret": "test",
				"authorizationUrl": "https://test.example.com/auth",
				"tokenUrl": "https://test.example.com/token"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:      strPtr(idpName),
				Definition: idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakIdentityProvider{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      idp.Name,
				Namespace: idp.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, idp))

		// Verify deleted from Kubernetes
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakIdentityProvider{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      idp.Name,
				Namespace: idp.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Identity provider was not deleted")
		t.Logf("Identity provider %s cleanup verified", idpName)
	})
}
