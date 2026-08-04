package controller

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// TestFindTopLevelGroupByName documents that the helper only matches against
// the top-level entries of the supplied slice. Callers (the KeycloakGroup
// reconciler) are expected to scope the slice to the right parent before
// calling it. Recursing into SubGroups would be incorrect across parents and
// is also unreliable on Keycloak 23+, where SubGroups is empty in the
// realm-wide listing.
func TestFindTopLevelGroupByName(t *testing.T) {
	ptr := func(s string) *string { return &s }
	groups := []keycloak.GroupRepresentation{
		{Name: ptr("alpha")},
		{Name: ptr("beta"), SubGroups: []keycloak.GroupRepresentation{{Name: ptr("nested")}}},
	}

	if got := findTopLevelGroupByName(groups, "beta"); got == nil || got.Name == nil || *got.Name != "beta" {
		t.Errorf("findTopLevelGroupByName(beta) = %+v, want beta", got)
	}
	if got := findTopLevelGroupByName(groups, "missing"); got != nil {
		t.Errorf("findTopLevelGroupByName(missing) = %+v, want nil", got)
	}
	if got := findTopLevelGroupByName(groups, "nested"); got != nil {
		t.Errorf("findTopLevelGroupByName must not recurse into SubGroups, got %+v", got)
	}
}

func nestedGroup(name, parent, realm string) *keycloakv1beta1.KeycloakGroup {
	g := &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
	if parent != "" {
		g.Spec.ParentGroupRef = &keycloakv1beta1.ResourceRef{Name: parent}
	}
	if realm != "" {
		g.Spec.RealmRef = &keycloakv1beta1.ResourceRef{Name: realm}
	}
	return g
}

// A nested group carries no realm ref, so the realm is only reachable by walking
// parentGroupRef to the root of the chain.
func TestResolveRealmOwner_WalksToRoot(t *testing.T) {
	root := nestedGroup("root", "", "my-realm")
	mid := nestedGroup("mid", "root", "")
	leaf := nestedGroup("leaf", "mid", "")

	r := &KeycloakGroupReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(root, mid, leaf).Build(),
	}

	owner, err := r.resolveRealmOwner(context.Background(), leaf)
	if err != nil {
		t.Fatalf("resolveRealmOwner: %v", err)
	}
	if owner.Name != "root" {
		t.Errorf("owner = %q, want root", owner.Name)
	}
	if owner.Spec.RealmRef == nil || owner.Spec.RealmRef.Name != "my-realm" {
		t.Errorf("owner realmRef = %+v, want my-realm", owner.Spec.RealmRef)
	}
}

func TestResolveRealmOwner_TopLevelIsItsOwnOwner(t *testing.T) {
	root := nestedGroup("root", "", "my-realm")
	r := &KeycloakGroupReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(root).Build(),
	}

	owner, err := r.resolveRealmOwner(context.Background(), root)
	if err != nil {
		t.Fatalf("resolveRealmOwner: %v", err)
	}
	if owner.Name != "root" {
		t.Errorf("owner = %q, want root", owner.Name)
	}
}

// A cycle must surface as an error rather than walk until the depth cap.
func TestResolveRealmOwner_CycleIsRejected(t *testing.T) {
	a := nestedGroup("a", "b", "")
	b := nestedGroup("b", "a", "")

	r := &KeycloakGroupReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(a, b).Build(),
	}

	_, err := r.resolveRealmOwner(context.Background(), a)
	if err == nil {
		t.Fatal("expected an error for a parentGroupRef cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should name the cycle, got %q", err)
	}
}

func TestResolveRealmOwner_MissingParentIsReported(t *testing.T) {
	leaf := nestedGroup("leaf", "gone", "")
	r := &KeycloakGroupReconciler{
		Client: fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(leaf).Build(),
	}

	_, err := r.resolveRealmOwner(context.Background(), leaf)
	if err == nil {
		t.Fatal("expected an error when the parent group is absent, got nil")
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("error should name the missing parent, got %q", err)
	}
}
