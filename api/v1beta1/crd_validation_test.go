package v1beta1

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	mapperName := "mapper"
	groupName := "group"
	componentName := "mapper"

	tests := []struct {
		name        string
		object      client.Object
		wantErrText string
	}{
		{
			name: "KeycloakClient accepts exactly one realm reference",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-valid", Namespace: namespace},
				Spec:       KeycloakClientSpec{ClientId: &clientID, RealmRef: &ResourceRef{Name: "realm"}},
			},
		},
		{
			name: "KeycloakClient rejects missing realm reference",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-missing", Namespace: namespace},
				Spec:       KeycloakClientSpec{ClientId: &clientID},
			},
			wantErrText: "exactly one of realmRef or clusterRealmRef must be set",
		},
		{
			name: "KeycloakClient rejects both realm references",
			object: &KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-both", Namespace: namespace},
				Spec: KeycloakClientSpec{
					ClientId:        &clientID,
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
					Name:           &mapperName,
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
			wantErrText: "exactly one of userRef, groupRef, or serviceAccountRef must be set",
		},
		{
			name: "KeycloakRoleMapping accepts a service account subject alone",
			object: &KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{Name: "mapping-sa-subject", Namespace: namespace},
				Spec: KeycloakRoleMappingSpec{
					Subject: RoleMappingSubject{ServiceAccountRef: clientRef},
					Role:    &RoleDefinition{Name: "role"},
				},
			},
		},
		{
			name: "KeycloakRoleMapping rejects a subject with no reference",
			object: &KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{Name: "mapping-no-subject", Namespace: namespace},
				Spec: KeycloakRoleMappingSpec{
					Role: &RoleDefinition{Name: "role"},
				},
			},
			wantErrText: "exactly one of userRef, groupRef, or serviceAccountRef must be set",
		},
		{
			name: "KeycloakGroup accepts a parent group alone",
			object: &KeycloakGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "group-nested", Namespace: namespace},
				Spec: KeycloakGroupSpec{
					Name:           &groupName,
					ParentGroupRef: &ResourceRef{Name: "parent"},
					Definition:     runtime.RawExtension{Raw: []byte(`{}`)},
				},
			},
		},
		{
			name: "KeycloakGroup rejects a parent group beside a realm reference",
			object: &KeycloakGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "group-both", Namespace: namespace},
				Spec: KeycloakGroupSpec{
					Name:           &groupName,
					RealmRef:       &ResourceRef{Name: "realm"},
					ParentGroupRef: &ResourceRef{Name: "parent"},
					Definition:     runtime.RawExtension{Raw: []byte(`{}`)},
				},
			},
			wantErrText: "exactly one of realmRef, clusterRealmRef, or parentGroupRef must be set",
		},
		{
			name: "KeycloakGroup rejects no parent reference",
			object: &KeycloakGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "group-none", Namespace: namespace},
				Spec: KeycloakGroupSpec{
					Name:       &groupName,
					Definition: runtime.RawExtension{Raw: []byte(`{}`)},
				},
			},
			wantErrText: "exactly one of realmRef, clusterRealmRef, or parentGroupRef must be set",
		},
		{
			name: "KeycloakComponent accepts a parent component reference",
			object: &KeycloakComponent{
				ObjectMeta: metav1.ObjectMeta{Name: "component-parent", Namespace: namespace},
				Spec: KeycloakComponentSpec{
					Name:               &componentName,
					ParentComponentRef: &ResourceRef{Name: "ldap"},
					Definition:         runtime.RawExtension{Raw: []byte(`{"providerId":"user-attribute-ldap-mapper"}`)},
				},
			},
		},
		{
			name: "KeycloakComponent rejects realm and parent component references together",
			object: &KeycloakComponent{
				ObjectMeta: metav1.ObjectMeta{Name: "component-realm-and-parent", Namespace: namespace},
				Spec: KeycloakComponentSpec{
					Name:               &componentName,
					RealmRef:           &ResourceRef{Name: "realm"},
					ParentComponentRef: &ResourceRef{Name: "ldap"},
					Definition:         runtime.RawExtension{Raw: []byte(`{"providerId":"user-attribute-ldap-mapper"}`)},
				},
			},
			wantErrText: "exactly one of realmRef, clusterRealmRef, or parentComponentRef must be set",
		},
		{
			name: "KeycloakComponent rejects no parent reference",
			object: &KeycloakComponent{
				ObjectMeta: metav1.ObjectMeta{Name: "component-none", Namespace: namespace},
				Spec: KeycloakComponentSpec{
					Name:       &componentName,
					Definition: runtime.RawExtension{Raw: []byte(`{"providerId":"ldap"}`)},
				},
			},
			wantErrText: "exactly one of realmRef, clusterRealmRef, or parentComponentRef must be set",
		},
		{
			name: "KeycloakComponent rejects duplicate configSecretRefs targets",
			object: &KeycloakComponent{
				ObjectMeta: metav1.ObjectMeta{Name: "component-dup-config-key", Namespace: namespace},
				Spec: KeycloakComponentSpec{
					Name:     &componentName,
					RealmRef: &ResourceRef{Name: "realm"},
					ConfigSecretRefs: []ConfigSecretRefMapping{
						{SecretName: "tls", Key: "tls.key", ConfigKey: "privateKey"},
						{SecretName: "tls", Key: "tls.crt", ConfigKey: "privateKey"},
					},
					Definition: runtime.RawExtension{Raw: []byte(`{"providerId":"rsa"}`)},
				},
			},
			wantErrText: "Duplicate value",
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

	// Regression: a namespaced KeycloakInstance must not be able to point its
	// Secret/ConfigMap refs at another namespace. The schema has no such field,
	// so the API server prunes it.
	t.Run("KeycloakInstance prunes cross-namespace refs", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(GroupVersion.WithKind("KeycloakInstance"))
		obj.SetName("instance-xns")
		obj.SetNamespace(namespace)
		obj.Object["spec"] = map[string]interface{}{
			"baseUrl": "http://attacker.example",
			"auth": map[string]interface{}{
				"passwordGrant": map[string]interface{}{
					"secretRef": map[string]interface{}{"name": "admin", "namespace": "victim"},
				},
			},
			"tls": map[string]interface{}{
				"caCert": map[string]interface{}{
					"secretRef": map[string]interface{}{"name": "ca", "namespace": "victim"},
				},
			},
		}
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("create instance: %v", err)
		}
		for _, path := range [][]string{
			{"spec", "auth", "passwordGrant", "secretRef", "namespace"},
			{"spec", "tls", "caCert", "secretRef", "namespace"},
		} {
			if _, found, _ := unstructured.NestedString(obj.Object, path...); found {
				t.Errorf("%s was persisted; it must be pruned by the schema", strings.Join(path, "."))
			}
		}
	})

	t.Run("KeycloakRealm managementPolicy defaults to Manage", func(t *testing.T) {
		realmName := "policy-default"
		realm := &KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-policy-default", Namespace: namespace},
			Spec: KeycloakRealmSpec{
				InstanceRef: &ResourceRef{Name: "instance"},
				RealmName:   &realmName,
				Definition:  runtime.RawExtension{Raw: []byte(`{}`)},
			},
		}
		if err := k8sClient.Create(ctx, realm); err != nil {
			t.Fatalf("create realm: %v", err)
		}
		if realm.Spec.ManagementPolicy != ManagementPolicyManage {
			t.Fatalf("managementPolicy = %q, want %q", realm.Spec.ManagementPolicy, ManagementPolicyManage)
		}

		// The policy is mutable in both directions.
		realm.Spec.ManagementPolicy = ManagementPolicyObserve
		if err := k8sClient.Update(ctx, realm); err != nil {
			t.Fatalf("switch to Observe: %v", err)
		}
		realm.Spec.ManagementPolicy = ManagementPolicyManage
		if err := k8sClient.Update(ctx, realm); err != nil {
			t.Fatalf("switch back to Manage: %v", err)
		}
	})

	t.Run("KeycloakRealm rejects an unknown managementPolicy", func(t *testing.T) {
		realmName := "policy-invalid"
		err := k8sClient.Create(ctx, &KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm-policy-invalid", Namespace: namespace},
			Spec: KeycloakRealmSpec{
				InstanceRef:      &ResourceRef{Name: "instance"},
				RealmName:        &realmName,
				ManagementPolicy: "ReadOnly",
				Definition:       runtime.RawExtension{Raw: []byte(`{}`)},
			},
		})
		if err == nil || !strings.Contains(err.Error(), "Unsupported value") {
			t.Fatalf("expected enum error, got %v", err)
		}
	})

	t.Run("ClusterKeycloakRealm accepts Observe with a definition", func(t *testing.T) {
		realmName := "policy-observe"
		realm := &ClusterKeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-realm-policy-observe"},
			Spec: ClusterKeycloakRealmSpec{
				ClusterInstanceRef: &ClusterResourceRef{Name: "instance"},
				RealmName:          &realmName,
				ManagementPolicy:   ManagementPolicyObserve,
				Definition:         runtime.RawExtension{Raw: []byte(`{}`)},
			},
		}
		if err := k8sClient.Create(ctx, realm); err != nil {
			t.Fatalf("create cluster realm: %v", err)
		}
	})

	t.Run("ClusterKeycloakRealm requires a definition in Observe mode", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(GroupVersion.WithKind("ClusterKeycloakRealm"))
		obj.SetName("cluster-realm-policy-no-definition")
		obj.Object["spec"] = map[string]interface{}{
			"clusterInstanceRef": map[string]interface{}{"name": "instance"},
			"realmName":          "policy-no-definition",
			"managementPolicy":   "Observe",
		}
		err := k8sClient.Create(ctx, obj)
		if err == nil || !strings.Contains(err.Error(), "spec.definition: Required value") {
			t.Fatalf("expected required definition error, got %v", err)
		}
	})

	t.Run("KeycloakComponent rejects a parentComponentRef change", func(t *testing.T) {
		component := &KeycloakComponent{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: "component-parent", Namespace: namespace}, component); err != nil {
			t.Fatalf("get component: %v", err)
		}
		component.Spec.ParentComponentRef = &ResourceRef{Name: "other-ldap"}
		err := k8sClient.Update(ctx, component)
		if err == nil || !strings.Contains(err.Error(), "spec.parentComponentRef is immutable") {
			t.Fatalf("expected immutability error, got %v", err)
		}
	})
}
