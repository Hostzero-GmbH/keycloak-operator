package v1beta1

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestCRDReferenceChoiceValidation(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("add keycloak scheme: %v", err)
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	})

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	const namespace = "crd-validation"
	if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	clientRef := &ResourceRef{Name: "client"}
	clientID := "client-id"

	tests := []struct {
		name        string
		object      client.Object
		wantErrText string
	}{
		{
			name: "KeycloakClient accepts exactly one realm reference",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-valid", Namespace: namespace},
				Spec:       KeycloakClientSpec{RealmRef: &ResourceRef{Name: "realm"}},
			},
		},
		{
			name: "KeycloakClient rejects missing realm reference",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-missing", Namespace: namespace},
				Spec:       KeycloakClientSpec{},
			},
			wantErrText: "exactly one of realmRef or clusterRealmRef must be set",
		},
		{
			name: "KeycloakClient rejects both realm references",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-both", Namespace: namespace},
				Spec: KeycloakClientSpec{
					RealmRef:        &ResourceRef{Name: "realm"},
					ClusterRealmRef: &ClusterResourceRef{Name: "cluster-realm"},
				},
			},
			wantErrText: "exactly one of realmRef or clusterRealmRef must be set",
		},
		{
			name: "KeycloakUser accepts exactly one user target",
			object: &KeycloakUser{
				ObjectMeta: metav1.ObjectMeta{Name: "user-valid", Namespace: namespace},
				Spec:       KeycloakUserSpec{ClientRef: clientRef},
			},
		},
		{
			name: "KeycloakUser rejects multiple user targets",
			object: &KeycloakUser{
				ObjectMeta: metav1.ObjectMeta{Name: "user-both", Namespace: namespace},
				Spec: KeycloakUserSpec{
					RealmRef:  &ResourceRef{Name: "realm"},
					ClientRef: clientRef,
				},
			},
			wantErrText: "exactly one of realmRef, clusterRealmRef, or clientRef must be set",
		},
		{
			name: "KeycloakProtocolMapper rejects both parents",
			object: &KeycloakProtocolMapper{
				ObjectMeta: metav1.ObjectMeta{Name: "mapper-both", Namespace: namespace},
				Spec: KeycloakProtocolMapperSpec{
					ClientRef:      clientRef,
					ClientScopeRef: &ResourceRef{Name: "scope"},
					Definition:     runtime.RawExtension{Raw: []byte(`{"name":"mapper"}`)},
				},
			},
			wantErrText: "exactly one of clientRef or clientScopeRef must be set",
		},
		{
			name: "KeycloakRoleMapping rejects both role sources",
			object: &KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{Name: "mapping-both-role", Namespace: namespace},
				Spec: KeycloakRoleMappingSpec{
					Subject: RoleMappingSubject{UserRef: &ResourceRef{Name: "user"}},
					Role:    &RoleDefinition{Name: "role"},
					RoleRef: &ResourceRef{Name: "role"},
				},
			},
			wantErrText: "exactly one of role or roleRef must be set",
		},
		{
			name: "KeycloakRoleMapping rejects both subject sources",
			object: &KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{Name: "mapping-both-subject", Namespace: namespace},
				Spec: KeycloakRoleMappingSpec{
					Subject: RoleMappingSubject{
						UserRef:  &ResourceRef{Name: "user"},
						GroupRef: &ResourceRef{Name: "group"},
					},
					Role: &RoleDefinition{Name: "role"},
				},
			},
			wantErrText: "exactly one of userRef or groupRef must be set",
		},
		{
			name: "RoleDefinition rejects both client selectors",
			object: &KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{Name: "mapping-both-client-role", Namespace: namespace},
				Spec: KeycloakRoleMappingSpec{
					Subject: RoleMappingSubject{UserRef: &ResourceRef{Name: "user"}},
					Role: &RoleDefinition{
						Name:      "role",
						ClientRef: clientRef,
						ClientID:  &clientID,
					},
				},
			},
			wantErrText: "at most one of clientRef or clientId may be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.object)
			if tt.wantErrText == "" {
				if err != nil {
					t.Fatalf("create valid object: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected create to fail with %q", tt.wantErrText)
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrText)
			}
		})
	}
}
