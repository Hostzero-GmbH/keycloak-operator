package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// resolveRealmFixtures builds one ready instance of each scope and one ready
// realm of each scope for each instance kind, so every reference combination
// can be resolved.
func resolveRealmFixtures() []client.Object {
	return []client.Object{
		mkSecret("admin", "kc", map[string]string{"username": "admin", "password": "pw"}),
		&keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: "kc"},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: "http://kc",
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
					},
				},
			},
			Status: keycloakv1beta1.KeycloakInstanceStatus{Ready: true, Version: "26.5.5"},
		},
		&keycloakv1beta1.ClusterKeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "central"},
			Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
				BaseUrl: "http://central-kc",
				Auth: keycloakv1beta1.ClusterAuthSpec{
					PasswordGrant: &keycloakv1beta1.ClusterPasswordGrantSpec{
						SecretRef: keycloakv1beta1.ClusterPasswordGrantSecretRefSpec{Name: "admin", Namespace: "kc"},
					},
				},
			},
			Status: keycloakv1beta1.ClusterKeycloakInstanceStatus{Ready: true, Version: "26.6.0"},
		},
		&keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-ns-instance", Namespace: "kc"},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: "kci"},
			},
			Status: keycloakv1beta1.KeycloakRealmStatus{Ready: true, RealmName: "demo1"},
		},
		// The combination from issue #135: a namespaced realm bound to a
		// ClusterKeycloakInstance.
		&keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-cluster-instance", Namespace: "demo"},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{Name: "central"},
			},
			Status: keycloakv1beta1.KeycloakRealmStatus{Ready: true, RealmName: "demo2"},
		},
		&keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-not-ready", Namespace: "demo"},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{Name: "central"},
			},
		},
		&keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "crealm-ns-instance"},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.NamespacedRef{Name: "kci", Namespace: "kc"},
			},
			Status: keycloakv1beta1.ClusterKeycloakRealmStatus{Ready: true, RealmName: "demo3"},
		},
		&keycloakv1beta1.ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "crealm-cluster-instance"},
			Spec: keycloakv1beta1.ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &keycloakv1beta1.ClusterResourceRef{Name: "central"},
			},
			Status: keycloakv1beta1.ClusterKeycloakRealmStatus{Ready: true, RealmName: "demo4"},
		},
	}
}

func TestResolveRealm(t *testing.T) {
	cases := []struct {
		name             string
		namespace        string
		realmRef         *keycloakv1beta1.ResourceRef
		clusterRealmRef  *keycloakv1beta1.ClusterResourceRef
		wantRealmName    string
		wantVersion      string
		wantClusterRealm bool
		wantErr          string
	}{
		{
			name:          "realmRef with instanceRef",
			namespace:     "kc",
			realmRef:      &keycloakv1beta1.ResourceRef{Name: "realm-ns-instance"},
			wantRealmName: "demo1",
			wantVersion:   "26.5.5",
		},
		{
			// Regression for issue #135.
			name:          "realmRef with clusterInstanceRef",
			namespace:     "demo",
			realmRef:      &keycloakv1beta1.ResourceRef{Name: "realm-cluster-instance"},
			wantRealmName: "demo2",
			wantVersion:   "26.6.0",
		},
		{
			name:             "clusterRealmRef with instanceRef",
			clusterRealmRef:  &keycloakv1beta1.ClusterResourceRef{Name: "crealm-ns-instance"},
			wantRealmName:    "demo3",
			wantVersion:      "26.5.5",
			wantClusterRealm: true,
		},
		{
			name:             "clusterRealmRef with clusterInstanceRef",
			clusterRealmRef:  &keycloakv1beta1.ClusterResourceRef{Name: "crealm-cluster-instance"},
			wantRealmName:    "demo4",
			wantVersion:      "26.6.0",
			wantClusterRealm: true,
		},
		{
			name:      "no refs",
			namespace: "demo",
			wantErr:   "either realmRef or clusterRealmRef must be specified",
		},
		{
			name:      "realm not ready",
			namespace: "demo",
			realmRef:  &keycloakv1beta1.ResourceRef{Name: "realm-not-ready"},
			wantErr:   "is not ready",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newAuthTestClient(t, resolveRealmFixtures()...)
			cm := keycloak.NewClientManager(logr.Discard())

			res, err := ResolveRealm(context.Background(), c, cm, tc.namespace, tc.realmRef, tc.clusterRealmRef)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got error %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Client == nil {
				t.Error("resolution returned nil Keycloak client")
			}
			if res.RealmName != tc.wantRealmName {
				t.Errorf("realm name: got %q, want %q", res.RealmName, tc.wantRealmName)
			}
			if res.Version != tc.wantVersion {
				t.Errorf("version: got %q, want %q", res.Version, tc.wantVersion)
			}
			if tc.wantClusterRealm {
				if res.ClusterRealm == nil || res.Realm != nil {
					t.Errorf("expected only ClusterRealm to be set, got Realm=%v ClusterRealm=%v", res.Realm, res.ClusterRealm)
				}
			} else {
				if res.Realm == nil || res.ClusterRealm != nil {
					t.Errorf("expected only Realm to be set, got Realm=%v ClusterRealm=%v", res.Realm, res.ClusterRealm)
				}
			}
		})
	}
}
