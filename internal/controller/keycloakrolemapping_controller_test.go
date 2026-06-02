package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := keycloakv1beta1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func TestResolveRole_InlineRole_Realm(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Role: &keycloakv1beta1.RoleDefinition{Name: "admin"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build(),
	}

	roleName, roleType, clientUUID, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roleName != "admin" || roleType != "realm" || clientUUID != "" {
		t.Errorf("got (%q, %q, %q), want (admin, realm, \"\")", roleName, roleType, clientUUID)
	}
}

func TestResolveRole_InlineRole_ClientRef_Ready(t *testing.T) {
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "my-client", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakClientStatus{Ready: true, ClientUUID: "uuid-999"},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Role: &keycloakv1beta1.RoleDefinition{
				Name:      "viewer",
				ClientRef: &keycloakv1beta1.ResourceRef{Name: "my-client"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(kcClient).Build(),
	}

	roleName, roleType, clientUUID, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roleName != "viewer" || roleType != "client" || clientUUID != "uuid-999" {
		t.Errorf("got (%q, %q, %q), want (viewer, client, uuid-999)", roleName, roleType, clientUUID)
	}
}

func TestResolveRole_InlineRole_ClientRef_NotReady(t *testing.T) {
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "my-client", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakClientStatus{Ready: false},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Role: &keycloakv1beta1.RoleDefinition{
				Name:      "viewer",
				ClientRef: &keycloakv1beta1.ResourceRef{Name: "my-client"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(kcClient).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error for unready client, got nil")
	}
}

func TestResolveRole_InlineRole_ClientRef_NotFound(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Role: &keycloakv1beta1.RoleDefinition{
				Name:      "viewer",
				ClientRef: &keycloakv1beta1.ResourceRef{Name: "missing-client"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error for missing client, got nil")
	}
}

func TestResolveRole_RoleRef_RealmRole(t *testing.T) {
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "my-role", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakRoleStatus{Ready: true, RoleName: "my-role"},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "my-role"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(role).Build(),
	}

	roleName, roleType, clientUUID, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roleName != "my-role" || roleType != "realm" || clientUUID != "" {
		t.Errorf("got (%q, %q, %q), want (my-role, realm, \"\")", roleName, roleType, clientUUID)
	}
}

func TestResolveRole_RoleRef_ClientRole(t *testing.T) {
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: "default"},
		Spec:       keycloakv1beta1.KeycloakRoleSpec{ClientRef: &keycloakv1beta1.ResourceRef{Name: "my-client"}},
		Status:     keycloakv1beta1.KeycloakRoleStatus{Ready: true, RoleName: "viewer"},
	}
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "my-client", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakClientStatus{Ready: true, ClientUUID: "abc-123"},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "viewer"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(role, kcClient).Build(),
	}

	roleName, roleType, clientUUID, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roleName != "viewer" || roleType != "client" || clientUUID != "abc-123" {
		t.Errorf("got (%q, %q, %q), want (viewer, client, abc-123)", roleName, roleType, clientUUID)
	}
}

func TestResolveRole_RoleRef_NotReady(t *testing.T) {
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "my-role", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakRoleStatus{Ready: false},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "my-role"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(role).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error for unready role, got nil")
	}
}

func TestResolveRole_RoleRef_NotFound(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "missing-role"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error for missing role, got nil")
	}
}

func TestResolveRole_RoleRef_ClientNotReady(t *testing.T) {
	// The KeycloakRole is ready but the client it references is not.
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "editor", Namespace: "default"},
		Spec:       keycloakv1beta1.KeycloakRoleSpec{ClientRef: &keycloakv1beta1.ResourceRef{Name: "my-client"}},
		Status:     keycloakv1beta1.KeycloakRoleStatus{Ready: true, RoleName: "editor"},
	}
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "my-client", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakClientStatus{Ready: false},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "editor"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(role, kcClient).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error when referenced client is not ready, got nil")
	}
}

func TestResolveRole_RoleRef_EmptyRoleName(t *testing.T) {
	// Ready=true but RoleName="" is a distinct error path from Ready=false.
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "my-role", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakRoleStatus{Ready: true, RoleName: ""},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "my-role"},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(role).Build(),
	}

	_, _, _, err := r.resolveRole(context.Background(), mapping, nil, "")
	if err == nil {
		t.Fatal("expected error for role with empty RoleName, got nil")
	}
}

func TestResolveSubject_UserRef_NotFound(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				UserRef: &keycloakv1beta1.ResourceRef{Name: "alice"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build(),
	}

	_, _, _, _, err := r.resolveSubject(context.Background(), mapping)
	if err == nil {
		t.Fatal("expected error for missing user, got nil")
	}
}

func TestResolveSubject_UserRef_NotReady(t *testing.T) {
	user := &keycloakv1beta1.KeycloakUser{
		ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakUserStatus{Ready: false},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				UserRef: &keycloakv1beta1.ResourceRef{Name: "alice"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(user).Build(),
	}

	subjectType, _, _, _, err := r.resolveSubject(context.Background(), mapping)
	if err == nil {
		t.Fatal("expected error for unready user, got nil")
	}
	if subjectType != "user" {
		t.Errorf("subjectType = %q, want \"user\"", subjectType)
	}
}

func TestResolveSubject_GroupRef_NotFound(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				GroupRef: &keycloakv1beta1.ResourceRef{Name: "devs"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build(),
	}

	_, _, _, _, err := r.resolveSubject(context.Background(), mapping)
	if err == nil {
		t.Fatal("expected error for missing group, got nil")
	}
}

func TestResolveSubject_GroupRef_NotReady(t *testing.T) {
	group := &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "devs", Namespace: "default"},
		Status:     keycloakv1beta1.KeycloakGroupStatus{Ready: false},
	}
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				GroupRef: &keycloakv1beta1.ResourceRef{Name: "devs"},
			},
		},
	}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(group).Build(),
	}

	subjectType, _, _, _, err := r.resolveSubject(context.Background(), mapping)
	if err == nil {
		t.Fatal("expected error for unready group, got nil")
	}
	if subjectType != "group" {
		t.Errorf("subjectType = %q, want \"group\"", subjectType)
	}
}

func TestFindRoleMappingsForUser(t *testing.T) {
	aliceMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "alice-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{UserRef: &keycloakv1beta1.ResourceRef{Name: "alice"}},
		},
	}
	bobMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "bob-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{UserRef: &keycloakv1beta1.ResourceRef{Name: "bob"}},
		},
	}
	alice := &keycloakv1beta1.KeycloakUser{ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "default"}}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(aliceMapping, bobMapping).Build(),
	}

	reqs := r.findRoleMappingsForUser(context.Background(), alice)
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	want := reconcile.Request{NamespacedName: types.NamespacedName{Name: "alice-mapping", Namespace: "default"}}
	if reqs[0] != want {
		t.Errorf("got %v, want %v", reqs[0], want)
	}
}

func TestFindRoleMappingsForUser_IgnoresGroupMappings(t *testing.T) {
	// A group mapping with the same name as the user must not be returned on a user event.
	groupMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "alice-group-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{GroupRef: &keycloakv1beta1.ResourceRef{Name: "alice"}},
		},
	}
	alice := &keycloakv1beta1.KeycloakUser{ObjectMeta: metav1.ObjectMeta{Name: "alice", Namespace: "default"}}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(groupMapping).Build(),
	}

	if reqs := r.findRoleMappingsForUser(context.Background(), alice); len(reqs) != 0 {
		t.Errorf("got %d requests, want 0", len(reqs))
	}
}

func TestFindRoleMappingsForGroup(t *testing.T) {
	devsMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "devs-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{GroupRef: &keycloakv1beta1.ResourceRef{Name: "devs"}},
		},
	}
	opsMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "ops-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{GroupRef: &keycloakv1beta1.ResourceRef{Name: "ops"}},
		},
	}
	devs := &keycloakv1beta1.KeycloakGroup{ObjectMeta: metav1.ObjectMeta{Name: "devs", Namespace: "default"}}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(devsMapping, opsMapping).Build(),
	}

	reqs := r.findRoleMappingsForGroup(context.Background(), devs)
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	want := reconcile.Request{NamespacedName: types.NamespacedName{Name: "devs-mapping", Namespace: "default"}}
	if reqs[0] != want {
		t.Errorf("got %v, want %v", reqs[0], want)
	}
}

func TestFindRoleMappingsForGroup_IgnoresUserMappings(t *testing.T) {
	// A user mapping with the same name as the group must not be returned on a group event.
	userMapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "devs-user-mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{UserRef: &keycloakv1beta1.ResourceRef{Name: "devs"}},
		},
	}
	devs := &keycloakv1beta1.KeycloakGroup{ObjectMeta: metav1.ObjectMeta{Name: "devs", Namespace: "default"}}

	r := &KeycloakRoleMappingReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(userMapping).Build(),
	}

	if reqs := r.findRoleMappingsForGroup(context.Background(), devs); len(reqs) != 0 {
		t.Errorf("got %d requests, want 0", len(reqs))
	}
}

func TestReconcile_InvalidSpec_NoSubject(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			// Subject is empty — neither UserRef nor GroupRef set
			Role: &keycloakv1beta1.RoleDefinition{Name: "admin"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&keycloakv1beta1.KeycloakRoleMapping{}).
		WithObjects(mapping).
		Build()
	r := &KeycloakRoleMappingReconciler{Client: cl}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "mapping", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := &keycloakv1beta1.KeycloakRoleMapping{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "mapping", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get mapping after reconcile: %v", err)
	}
	if got.Status.Ready {
		t.Error("Status.Ready = true, want false")
	}
	if got.Status.Status != "InvalidSpec" {
		t.Errorf("Status.Status = %q, want \"InvalidSpec\"", got.Status.Status)
	}
}

func TestReconcile_InvalidSpec_NoRole(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				UserRef: &keycloakv1beta1.ResourceRef{Name: "alice"},
			},
			// Neither Role nor RoleRef set
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&keycloakv1beta1.KeycloakRoleMapping{}).
		WithObjects(mapping).
		Build()
	r := &KeycloakRoleMappingReconciler{Client: cl}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "mapping", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := &keycloakv1beta1.KeycloakRoleMapping{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "mapping", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get mapping after reconcile: %v", err)
	}
	if got.Status.Ready {
		t.Error("Status.Ready = true, want false")
	}
	if got.Status.Status != "InvalidSpec" {
		t.Errorf("Status.Status = %q, want \"InvalidSpec\"", got.Status.Status)
	}
}

func TestReconcile_AddsFinalizerOnFirstReconcile(t *testing.T) {
	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "mapping", Namespace: "default"},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				UserRef: &keycloakv1beta1.ResourceRef{Name: "alice"},
			},
			RoleRef: &keycloakv1beta1.ResourceRef{Name: "my-role"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&keycloakv1beta1.KeycloakRoleMapping{}).
		WithObjects(mapping).
		Build()
	r := &KeycloakRoleMappingReconciler{Client: cl}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "mapping", Namespace: "default"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := &keycloakv1beta1.KeycloakRoleMapping{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "mapping", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get mapping after reconcile: %v", err)
	}
	if !controllerutil.ContainsFinalizer(got, FinalizerName) {
		t.Errorf("finalizer %q was not added", FinalizerName)
	}
}
