package controller

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func configSecretScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := newScheme(t)
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme corev1: %v", err)
	}
	return s
}

func TestApplyConfigSecret(t *testing.T) {
	t.Parallel()

	secret := mkSecret("ldap-credentials", "ns", map[string]string{
		"bindCredential": "s3cret",
	})
	cl := fake.NewClientBuilder().WithScheme(configSecretScheme(t)).WithObjects(secret).Build()

	t.Run("nil ref leaves definition unchanged", func(t *testing.T) {
		in := json.RawMessage(`{"config":{"enabled":["true"]}}`)
		got, err := applyConfigSecret(context.Background(), cl, "ns", nil, in, true)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(in) {
			t.Errorf("got %s, want original", got)
		}
	})

	t.Run("missing secret returns error", func(t *testing.T) {
		_, err := applyConfigSecret(context.Background(), cl, "ns", &keycloakv1beta1.ConfigSecretRef{Name: "missing"}, json.RawMessage(`{}`), false)
		if err == nil {
			t.Fatal("expected error for missing secret")
		}
	})

	t.Run("merges secret keys wrapAsList", func(t *testing.T) {
		got, err := applyConfigSecret(context.Background(), cl, "ns", &keycloakv1beta1.ConfigSecretRef{Name: "ldap-credentials"}, json.RawMessage(`{"config":{"enabled":["true"]}}`), true)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(got, &m); err != nil {
			t.Fatal(err)
		}
		cfg := m["config"].(map[string]interface{})
		bind := cfg["bindCredential"].([]interface{})
		if len(bind) != 1 || bind[0] != "s3cret" {
			t.Errorf("bindCredential: got %v, want [s3cret]", cfg["bindCredential"])
		}
	})
}

func TestFindForConfigSecret(t *testing.T) {
	t.Parallel()

	matching := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "ldap", Namespace: "ns"},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			ConfigSecretRef: &keycloakv1beta1.ConfigSecretRef{Name: "ldap-credentials"},
		},
	}
	other := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "rsa", Namespace: "ns"},
	}
	wrongNS := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "other-ldap", Namespace: "other"},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			ConfigSecretRef: &keycloakv1beta1.ConfigSecretRef{Name: "ldap-credentials"},
		},
	}
	secret := mkSecret("ldap-credentials", "ns", map[string]string{"bindCredential": "x"})
	cl := fake.NewClientBuilder().WithScheme(configSecretScheme(t)).WithObjects(matching, other, wrongNS, secret).Build()

	reqs := findForConfigSecret(context.Background(), cl, secret, &keycloakv1beta1.KeycloakComponentList{}, func(o client.Object) *keycloakv1beta1.ConfigSecretRef {
		return o.(*keycloakv1beta1.KeycloakComponent).Spec.ConfigSecretRef
	})
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	if reqs[0].NamespacedName != (types.NamespacedName{Name: "ldap", Namespace: "ns"}) {
		t.Errorf("got %s, want ns/ldap", reqs[0].NamespacedName)
	}
}
