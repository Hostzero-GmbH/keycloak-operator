package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// TestKeycloakUserSecretE2E covers user credential secrets
// (https://github.com/Hostzero-GmbH/keycloak-operator/issues/133).
//
// KeycloakUser.spec.userSecret was advertised by the CRD but never implemented;
// it has been removed. Credential secrets are managed by KeycloakUserCredential.
func TestKeycloakUserSecretE2E(t *testing.T) {
	skipIfNoCluster(t)

	// Guard against the dead API coming back: the KeycloakUser CRD must not
	// advertise a userSecret field it does not implement.
	t.Run("UserSecretRemovedFromCRD", func(t *testing.T) {
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apiextensions.k8s.io",
			Version: "v1",
			Kind:    "CustomResourceDefinition",
		})
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name: "keycloakusers.keycloak.hostzero.com",
		}, crd))

		versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
		require.NoError(t, err)
		require.True(t, found, "CRD should have versions")
		for _, v := range versions {
			version := v.(map[string]interface{})
			specProps, found, err := unstructured.NestedMap(version,
				"schema", "openAPIV3Schema", "properties", "spec", "properties")
			require.NoError(t, err)
			require.True(t, found, "CRD version should have a spec schema")
			require.NotContains(t, specProps, "userSecret",
				"KeycloakUser CRD must not advertise spec.userSecret (version %s)", version["name"])
		}
	})

	// The supported path: KeycloakUserCredential with create=true generates a
	// password, stores it in a secret, and sets it in Keycloak.
	t.Run("GeneratedCredentialsWork", func(t *testing.T) {
		instanceName, _ := getOrCreateInstance(t)
		realmName := createTestRealm(t, instanceName, "usersecret")

		userName := fmt.Sprintf("gen-pw-user-%d", time.Now().UnixNano())
		// Complete profile: pending required actions (e.g. unverified email)
		// fail direct grants with "Account is not fully set up".
		userDef := rawJSON(fmt.Sprintf(`{
			"email": "%s@example.com",
			"emailVerified": true,
			"firstName": "Gen",
			"lastName": "Password",
			"enabled": true
		}`, userName))
		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Username:   strPtr(userName),
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser did not become ready")

		secretName := userName + "-credentials"
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName + "-cred",
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName: secretName,
					Create:     true,
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcCred)
		})

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUserCredential did not become ready")

		secret := &corev1.Secret{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: testNamespace,
		}, secret), "credential secret was not created")
		require.Equal(t, userName, string(secret.Data["username"]))
		require.NotEmpty(t, secret.Data["password"], "secret should contain a generated password")

		// Verify the generated password is actually set in Keycloak by logging in
		if !canConnectToKeycloak() {
			t.Log("skipping direct login verification - Keycloak not reachable from test environment")
			return
		}
		keycloakURL := os.Getenv("KEYCLOAK_URL")
		if keycloakURL == "" {
			keycloakURL = "http://keycloak.keycloak.svc.cluster.local"
		}
		userClient := keycloak.NewClient(keycloak.Config{
			BaseURL:  keycloakURL,
			Realm:    realmName,
			Username: userName,
			Password: string(secret.Data["password"]),
		}, ctrl.Log.WithName("test-user-login"))
		require.NoError(t, userClient.Ping(ctx), "login with the generated password should succeed")
	})
}
