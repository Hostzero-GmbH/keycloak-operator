package export

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestGetKeycloakConfigFromCluster_FromInstance(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "keycloak"},
		Data: map[string][]byte{
			"client-id":     []byte("operator"),
			"client-secret": []byte("s3cret"),
		},
	}
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "keycloak", Namespace: "keycloak"},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: "https://kc.example.com",
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: "admin"},
				},
			},
		},
	}

	opts := &Options{FromInstance: "keycloak", Namespace: "keycloak"}
	cfg, err := opts.getKeycloakConfigFromCluster(context.Background(), newFakeClient(secret, instance), logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://kc.example.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.ClientID != "operator" || cfg.ClientSecret != "s3cret" {
		t.Errorf("client credentials = %q/%q", cfg.ClientID, cfg.ClientSecret)
	}
}

func TestGetKeycloakConfigFromCluster_FromClusterInstance(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "keycloak"},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("pw"),
		},
	}
	instance := &keycloakv1beta1.ClusterKeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "keycloak"},
		Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
			BaseUrl: "https://kc.example.com",
			Auth: keycloakv1beta1.ClusterAuthSpec{
				PasswordGrant: &keycloakv1beta1.ClusterPasswordGrantSpec{
					SecretRef: keycloakv1beta1.ClusterPasswordGrantSecretRefSpec{Name: "admin", Namespace: "keycloak"},
				},
			},
		},
	}

	opts := &Options{FromClusterInstance: "keycloak"}
	cfg, err := opts.getKeycloakConfigFromCluster(context.Background(), newFakeClient(secret, instance), logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://kc.example.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Username != "admin" || cfg.Password != "pw" {
		t.Errorf("credentials = %q/%q", cfg.Username, cfg.Password)
	}
}

func TestGetKeycloakConfigFromCluster_InstanceNotFound(t *testing.T) {
	opts := &Options{FromInstance: "missing", Namespace: "keycloak"}
	if _, err := opts.getKeycloakConfigFromCluster(context.Background(), newFakeClient(), logr.Discard()); err == nil {
		t.Fatal("expected error for missing KeycloakInstance")
	}
}
