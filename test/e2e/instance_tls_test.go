package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TestKeycloakInstanceTLSE2E covers spec.tls handling. The kind dev cluster
// serves Keycloak over plain HTTP, so the relevant assertions are: (a) admission
// accepts/rejects the spec as expected, (b) tls is a no-op for an http base URL
// (insecureSkipVerify still allows the connection to succeed), and (c) a
// missing caCert source surfaces in status.
func TestKeycloakInstanceTLSE2E(t *testing.T) {
	skipIfNoCluster(t)

	t.Run("InsecureSkipVerify_IsAccepted_AndConnects", func(t *testing.T) {
		secretName := fmt.Sprintf("creds-tls-skip-%d", time.Now().UnixNano())
		require.NoError(t, k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			StringData: map[string]string{"username": "admin", "password": "admin"},
		}))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace}})
		})

		instanceName := fmt.Sprintf("tls-skip-%d", time.Now().UnixNano())
		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: getKeycloakURL(),
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: secretName},
					},
				},
				TLS: &keycloakv1beta1.TLSSpec{InsecureSkipVerify: true},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		require.Eventually(t, func() bool {
			updated := &keycloakv1beta1.KeycloakInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updated); err != nil {
				return false
			}
			return updated.Status.Ready
		}, 30*time.Second, 1*time.Second, "instance with insecureSkipVerify=true should become ready")
	})

	t.Run("CACertFromConfigMap_IsAccepted", func(t *testing.T) {
		// We don't have a real CA bundle for the kind Keycloak — Keycloak is on
		// HTTP — but the CR shape must be accepted and the operator must
		// resolve the ConfigMap without surfacing a Get/key error in status.
		cmName := fmt.Sprintf("ca-cm-%d", time.Now().UnixNano())
		require.NoError(t, k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: testNamespace},
			Data: map[string]string{
				"ca.crt": "-----BEGIN CERTIFICATE-----\nMIIBaTCCAQ6gAwIBAgIBATAKBggqhkjOPQQDAjAQMQ4wDAYDVQQDEwVkdW1teTAe\n-----END CERTIFICATE-----\n",
			},
		}))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: testNamespace}})
		})

		secretName := fmt.Sprintf("creds-tls-cm-%d", time.Now().UnixNano())
		require.NoError(t, k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			StringData: map[string]string{"username": "admin", "password": "admin"},
		}))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace}})
		})

		instanceName := fmt.Sprintf("tls-cm-%d", time.Now().UnixNano())
		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: getKeycloakURL(),
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: secretName},
					},
				},
				TLS: &keycloakv1beta1.TLSSpec{
					CACert: &keycloakv1beta1.CACertSource{
						ConfigMapRef: &keycloakv1beta1.CACertConfigMapRefSpec{Name: cmName},
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		// The operator should at least process the CR; we expect Ready because
		// Keycloak is HTTP and the dummy CA bundle is unused.
		require.Eventually(t, func() bool {
			updated := &keycloakv1beta1.KeycloakInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updated); err != nil {
				return false
			}
			return updated.Status.Ready
		}, 30*time.Second, 1*time.Second, "instance with caCert configMapRef should become ready")
	})

	t.Run("MissingCACertSecret_SurfacesInStatus", func(t *testing.T) {
		secretName := fmt.Sprintf("creds-tls-missing-%d", time.Now().UnixNano())
		require.NoError(t, k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			StringData: map[string]string{"username": "admin", "password": "admin"},
		}))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace}})
		})

		instanceName := fmt.Sprintf("tls-missing-%d", time.Now().UnixNano())
		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: getKeycloakURL(),
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: secretName},
					},
				},
				TLS: &keycloakv1beta1.TLSSpec{
					CACert: &keycloakv1beta1.CACertSource{
						SecretRef: &keycloakv1beta1.CACertSecretRefSpec{Name: "does-not-exist"},
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, instance))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, instance)
		})

		require.Eventually(t, func() bool {
			updated := &keycloakv1beta1.KeycloakInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updated); err != nil {
				return false
			}
			return !updated.Status.Ready && updated.Status.Message != ""
		}, 30*time.Second, 1*time.Second, "missing caCert secret should surface in status")
	})

	t.Run("CACertWithBothSources_IsRejectedByAdmission", func(t *testing.T) {
		secretName := fmt.Sprintf("creds-tls-both-%d", time.Now().UnixNano())
		require.NoError(t, k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			StringData: map[string]string{"username": "admin", "password": "admin"},
		}))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace}})
		})

		instanceName := fmt.Sprintf("tls-both-%d", time.Now().UnixNano())
		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: getKeycloakURL(),
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: secretName},
					},
				},
				TLS: &keycloakv1beta1.TLSSpec{
					CACert: &keycloakv1beta1.CACertSource{
						SecretRef:    &keycloakv1beta1.CACertSecretRefSpec{Name: "a"},
						ConfigMapRef: &keycloakv1beta1.CACertConfigMapRefSpec{Name: "b"},
					},
				},
			},
		}
		err := k8sClient.Create(ctx, instance)
		require.Error(t, err, "API server should reject caCert with both sources set")
		t.Logf("admission correctly rejected: %v", err)
	})
}
