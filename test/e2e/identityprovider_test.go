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

func TestKeycloakIdentityProviderE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "idp")

	t.Run("OIDCIdentityProvider", func(t *testing.T) {
		idpName := fmt.Sprintf("test-oidc-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(fmt.Sprintf(`{
			"alias": "%s",
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
		}`, idpName))

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
		idpDef := rawJSON(fmt.Sprintf(`{
			"alias": "%s",
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
		}`, idpName))

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
		idpDef := rawJSON(fmt.Sprintf(`{
			"alias": "%s",
			"displayName": "GitHub",
			"providerId": "github",
			"enabled": true,
			"trustEmail": true,
			"config": {
				"clientId": "github-client-id",
				"clientSecret": "github-client-secret",
				"defaultScope": "read:user user:email"
			}
		}`, idpName))

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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

	t.Run("IdentityProviderCleanup", func(t *testing.T) {
		idpName := fmt.Sprintf("cleanup-idp-%d", time.Now().UnixNano())
		idpDef := rawJSON(fmt.Sprintf(`{
			"alias": "%s",
			"providerId": "oidc",
			"enabled": true,
			"config": {
				"clientId": "test",
				"clientSecret": "test",
				"authorizationUrl": "https://test.example.com/auth",
				"tokenUrl": "https://test.example.com/token"
			}
		}`, idpName))

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
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
