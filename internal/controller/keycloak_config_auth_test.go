package controller

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func newAuthTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := keycloakv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("add keycloakv1beta1 scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func mkSecret(name, namespace string, data map[string]string) *corev1.Secret {
	d := map[string][]byte{}
	for k, v := range data {
		d[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       d,
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant(t *testing.T) {
	secret := mkSecret("admin", "kc", map[string]string{
		"username": "admin",
		"password": "s3cret",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: "http://kc",
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
		},
	}

	c := newAuthTestClient(t, secret, instance)
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), c, instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Username != "admin" || cfg.Password != "s3cret" {
		t.Errorf("got user=%q pass=%q, want admin/s3cret", cfg.Username, cfg.Password)
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" {
		t.Errorf("client fields should be empty for password grant, got %q/%q", cfg.ClientID, cfg.ClientSecret)
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant_InlineUsernameOverrides(t *testing.T) {
	// Secret carries a stale username; inline value must win and the secret
	// key need not be present.
	secret := mkSecret("admin", "kc", map[string]string{
		"password": "s3cret",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: "http://kc",
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					Username:  strPtr("alice"),
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
		},
	}

	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Username != "alice" {
		t.Errorf("inline username should win, got %q", cfg.Username)
	}
	if cfg.Password != "s3cret" {
		t.Errorf("password: got %q want s3cret", cfg.Password)
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant_MissingUsernameKey(t *testing.T) {
	secret := mkSecret("admin", "kc", map[string]string{"password": "p"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "username key") {
		t.Fatalf("expected username-key error, got %v", err)
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant_MissingPasswordKey(t *testing.T) {
	secret := mkSecret("admin", "kc", map[string]string{"username": "admin"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "password key") {
		t.Fatalf("expected password-key error, got %v", err)
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant_CustomKeys(t *testing.T) {
	secret := mkSecret("admin", "kc", map[string]string{
		"u": "admin",
		"p": "x",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{
						Name:        "admin",
						UsernameKey: "u",
						PasswordKey: "p",
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Username != "admin" || cfg.Password != "x" {
		t.Errorf("got %q/%q want admin/x", cfg.Username, cfg.Password)
	}
}

func TestGetKeycloakConfigFromInstance_PasswordGrant_SecretInOtherNamespace(t *testing.T) {
	secret := mkSecret("admin", "secrets", map[string]string{
		"username": "admin",
		"password": "p",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{
						Name:      "admin",
						Namespace: strPtr("secrets"),
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Username != "admin" {
		t.Errorf("got %q want admin", cfg.Username)
	}
}

func TestGetKeycloakConfigFromInstance_ClientCredentials(t *testing.T) {
	secret := mkSecret("svc", "kc", map[string]string{
		"client-id":     "kc-op",
		"client-secret": "topsecret",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: "svc"},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "kc-op" || cfg.ClientSecret != "topsecret" {
		t.Errorf("got %q/%q want kc-op/topsecret", cfg.ClientID, cfg.ClientSecret)
	}
	if cfg.Username != "" || cfg.Password != "" {
		t.Errorf("password fields should be empty for client_credentials, got %q/%q", cfg.Username, cfg.Password)
	}
}

func TestGetKeycloakConfigFromInstance_ClientCredentials_InlineIDOverrides(t *testing.T) {
	// Secret omits client-id; inline value must satisfy.
	secret := mkSecret("svc", "kc", map[string]string{
		"client-secret": "topsecret",
	})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					ClientID:  strPtr("kc-op-inline"),
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: "svc"},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "kc-op-inline" {
		t.Errorf("inline clientId should win, got %q", cfg.ClientID)
	}
}

func TestGetKeycloakConfigFromInstance_ClientCredentials_MissingClientSecret(t *testing.T) {
	secret := mkSecret("svc", "kc", map[string]string{"client-id": "kc-op"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: "svc"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "client secret key") {
		t.Fatalf("expected client-secret-key error, got %v", err)
	}
}

func TestGetKeycloakConfigFromInstance_ClientCredentials_MissingClientId(t *testing.T) {
	secret := mkSecret("svc", "kc", map[string]string{"client-secret": "topsecret"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: "svc"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "client id key") {
		t.Fatalf("expected client-id-key error, got %v", err)
	}
}

func TestGetKeycloakConfigFromInstance_NoAuth(t *testing.T) {
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec:       keycloakv1beta1.KeycloakInstanceSpec{BaseUrl: "http://kc"},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, instance), instance)
	if err == nil {
		t.Fatal("expected error when neither grant is set")
	}
}

func TestGetKeycloakConfigFromInstance_RealmDefault(t *testing.T) {
	secret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Realm: strPtr("custom"),
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Realm != "custom" {
		t.Errorf("realm: got %q want custom", cfg.Realm)
	}
}

func TestGetKeycloakConfigFromClusterInstance_PasswordGrant(t *testing.T) {
	secret := mkSecret("admin", "secrets", map[string]string{"username": "admin", "password": "p"})
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "ckci"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			Auth: keycloakv1beta1.ClusterAuthSpec{
				PasswordGrant: &keycloakv1beta1.ClusterPasswordGrantSpec{
					SecretRef: keycloakv1beta1.ClusterPasswordGrantSecretRefSpec{
						Name:      "admin",
						Namespace: "secrets",
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromClusterInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Username != "admin" || cfg.Password != "p" {
		t.Errorf("got %q/%q want admin/p", cfg.Username, cfg.Password)
	}
}

func TestGetKeycloakConfigFromClusterInstance_ClientCredentials(t *testing.T) {
	secret := mkSecret("svc", "secrets", map[string]string{
		"client-id":     "kc-op",
		"client-secret": "topsecret",
	})
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "ckci"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			Auth: keycloakv1beta1.ClusterAuthSpec{
				ClientCredentials: &keycloakv1beta1.ClusterClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClusterClientCredentialsSecretRefSpec{
						Name:      "svc",
						Namespace: "secrets",
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromClusterInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "kc-op" || cfg.ClientSecret != "topsecret" {
		t.Errorf("got %q/%q want kc-op/topsecret", cfg.ClientID, cfg.ClientSecret)
	}
}

const testCAPEM = "-----BEGIN CERTIFICATE-----\nfake-ca-bytes\n-----END CERTIFICATE-----\n"

func mkConfigMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       data,
	}
}

func TestGetKeycloakConfigFromInstance_TLS_CACertFromSecret(t *testing.T) {
	adminSecret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	caSecret := mkSecret("ca", "kc", map[string]string{"ca.crt": testCAPEM})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: "https://kc",
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
			TLS: &keycloakv1beta1.TLSSpec{
				CACert: &keycloakv1beta1.CACertSource{
					SecretRef: &keycloakv1beta1.CACertSecretRefSpec{Name: "ca"},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, adminSecret, caSecret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CACert != testCAPEM {
		t.Errorf("CACert: got %q want %q", cfg.CACert, testCAPEM)
	}
	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should default to false")
	}
}

func TestGetKeycloakConfigFromInstance_TLS_CACertFromConfigMap(t *testing.T) {
	adminSecret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	caCM := mkConfigMap("ca", "kc", map[string]string{"ca.crt": testCAPEM})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
			TLS: &keycloakv1beta1.TLSSpec{
				CACert: &keycloakv1beta1.CACertSource{
					ConfigMapRef: &keycloakv1beta1.CACertConfigMapRefSpec{Name: "ca"},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, adminSecret, caCM, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CACert != testCAPEM {
		t.Errorf("CACert: got %q want %q", cfg.CACert, testCAPEM)
	}
}

func TestGetKeycloakConfigFromInstance_TLS_InsecureSkipVerify(t *testing.T) {
	adminSecret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
			TLS: &keycloakv1beta1.TLSSpec{InsecureSkipVerify: true},
		},
	}
	cfg, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, adminSecret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestGetKeycloakConfigFromInstance_TLS_CACertMissingSource(t *testing.T) {
	adminSecret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
			TLS: &keycloakv1beta1.TLSSpec{
				CACert: &keycloakv1beta1.CACertSource{
					SecretRef: &keycloakv1beta1.CACertSecretRefSpec{Name: "missing"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, adminSecret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "caCert secret") {
		t.Fatalf("expected caCert secret error, got %v", err)
	}
}

func TestGetKeycloakConfigFromInstance_TLS_CACertMissingKey(t *testing.T) {
	adminSecret := mkSecret("admin", "kc", map[string]string{"username": "u", "password": "p"})
	caSecret := mkSecret("ca", "kc", map[string]string{"other": testCAPEM})
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			Auth: keycloakv1beta1.AuthSpec{
				PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
					SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
				},
			},
			TLS: &keycloakv1beta1.TLSSpec{
				CACert: &keycloakv1beta1.CACertSource{
					SecretRef: &keycloakv1beta1.CACertSecretRefSpec{Name: "ca"},
				},
			},
		},
	}
	_, err := GetKeycloakConfigFromInstance(context.Background(), newAuthTestClient(t, adminSecret, caSecret, instance), instance)
	if err == nil || !strings.Contains(err.Error(), "caCert key") {
		t.Fatalf("expected caCert key error, got %v", err)
	}
}

func TestGetKeycloakConfigFromClusterInstance_TLS_CACertFromConfigMap(t *testing.T) {
	adminSecret := mkSecret("admin", "secrets", map[string]string{"username": "u", "password": "p"})
	caCM := mkConfigMap("ca", "secrets", map[string]string{"ca.crt": testCAPEM})
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "ckci"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			Auth: keycloakv1beta1.ClusterAuthSpec{
				PasswordGrant: &keycloakv1beta1.ClusterPasswordGrantSpec{
					SecretRef: keycloakv1beta1.ClusterPasswordGrantSecretRefSpec{
						Name: "admin", Namespace: "secrets",
					},
				},
			},
			TLS: &keycloakv1beta1.ClusterTLSSpec{
				CACert: &keycloakv1beta1.ClusterCACertSource{
					ConfigMapRef: &keycloakv1beta1.ClusterCACertConfigMapRefSpec{
						Name: "ca", Namespace: "secrets",
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromClusterInstance(context.Background(), newAuthTestClient(t, adminSecret, caCM, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CACert != testCAPEM {
		t.Errorf("CACert: got %q want %q", cfg.CACert, testCAPEM)
	}
}

func TestGetKeycloakConfigFromClusterInstance_TLS_InsecureSkipVerify(t *testing.T) {
	adminSecret := mkSecret("admin", "secrets", map[string]string{"username": "u", "password": "p"})
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "ckci"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			Auth: keycloakv1beta1.ClusterAuthSpec{
				PasswordGrant: &keycloakv1beta1.ClusterPasswordGrantSpec{
					SecretRef: keycloakv1beta1.ClusterPasswordGrantSecretRefSpec{
						Name: "admin", Namespace: "secrets",
					},
				},
			},
			TLS: &keycloakv1beta1.ClusterTLSSpec{InsecureSkipVerify: true},
		},
	}
	cfg, err := GetKeycloakConfigFromClusterInstance(context.Background(), newAuthTestClient(t, adminSecret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestGetKeycloakConfigFromClusterInstance_ClientCredentials_InlineIDOverrides(t *testing.T) {
	secret := mkSecret("svc", "secrets", map[string]string{"client-secret": "topsecret"})
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "ckci"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			Auth: keycloakv1beta1.ClusterAuthSpec{
				ClientCredentials: &keycloakv1beta1.ClusterClientCredentialsSpec{
					ClientID: strPtr("kc-op-inline"),
					SecretRef: keycloakv1beta1.ClusterClientCredentialsSecretRefSpec{
						Name:      "svc",
						Namespace: "secrets",
					},
				},
			},
		},
	}
	cfg, err := GetKeycloakConfigFromClusterInstance(context.Background(), newAuthTestClient(t, secret, instance), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "kc-op-inline" {
		t.Errorf("got %q want kc-op-inline", cfg.ClientID)
	}
}
