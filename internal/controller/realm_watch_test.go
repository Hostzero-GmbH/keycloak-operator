package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func flowRefs(o client.Object) (*keycloakv1beta1.ResourceRef, *keycloakv1beta1.ClusterResourceRef) {
	f := o.(*keycloakv1beta1.KeycloakAuthenticationFlow)
	return f.Spec.RealmRef, f.Spec.ClusterRealmRef
}

func newFlowList() client.ObjectList { return &keycloakv1beta1.KeycloakAuthenticationFlowList{} }

func TestFindForRealm(t *testing.T) {
	t.Parallel()

	matching := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "browser", Namespace: "ns"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm"}},
	}
	otherRealm := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "other-realm"}},
	}
	wrongNS := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "browser", Namespace: "elsewhere"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm"}},
	}
	clusterRef := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-flow", Namespace: "ns"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "realm"}},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(matching, otherRealm, wrongNS, clusterRef).Build()

	realm := &keycloakv1beta1.KeycloakRealm{ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: "ns"}}
	reqs := findForRealm(cl, newFlowList, flowRefs)(context.Background(), realm)
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1: %v", len(reqs), reqs)
	}
	if reqs[0].NamespacedName != (types.NamespacedName{Name: "browser", Namespace: "ns"}) {
		t.Errorf("got %s, want ns/browser", reqs[0].NamespacedName)
	}
}

func TestFindForClusterRealm(t *testing.T) {
	t.Parallel()

	nsA := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns-a"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
	}
	nsB := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns-b"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
	}
	namespacedRef := &keycloakv1beta1.KeycloakAuthenticationFlow{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns-a"},
		Spec:       keycloakv1beta1.KeycloakAuthenticationFlowSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "shared"}},
	}
	cl := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(nsA, nsB, namespacedRef).Build()

	realm := &keycloakv1beta1.ClusterKeycloakRealm{ObjectMeta: metav1.ObjectMeta{Name: "shared"}}
	reqs := findForClusterRealm(cl, newFlowList, flowRefs)(context.Background(), realm)
	if len(reqs) != 2 {
		t.Fatalf("got %d requests, want 2: %v", len(reqs), reqs)
	}
	got := map[types.NamespacedName]bool{}
	for _, r := range reqs {
		got[r.NamespacedName] = true
	}
	for _, want := range []types.NamespacedName{{Name: "a", Namespace: "ns-a"}, {Name: "b", Namespace: "ns-b"}} {
		if !got[want] {
			t.Errorf("missing request for %s", want)
		}
	}
}

func TestRealmReadinessChanged(t *testing.T) {
	t.Parallel()

	realm := func(ready bool, name string) *keycloakv1beta1.KeycloakRealm {
		return &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: "ns"},
			Status:     keycloakv1beta1.KeycloakRealmStatus{Ready: ready, RealmName: name},
		}
	}
	p := realmReadinessChanged()

	cases := []struct {
		name     string
		old, new *keycloakv1beta1.KeycloakRealm
		want     bool
	}{
		{"not ready to ready", realm(false, "r"), realm(true, "r"), true},
		{"ready to not ready", realm(true, "r"), realm(false, "r"), true},
		{"realm name resolved", realm(false, ""), realm(false, "r"), true},
		{"unrelated status write", realm(true, "r"), realm(true, "r"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Update(event.UpdateEvent{ObjectOld: tc.old, ObjectNew: tc.new}); got != tc.want {
				t.Errorf("Update: got %v, want %v", got, tc.want)
			}
		})
	}

	if p.Create(event.CreateEvent{Object: realm(true, "r")}) {
		t.Error("Create should not enqueue dependents")
	}
	if !p.Delete(event.DeleteEvent{Object: realm(true, "r")}) {
		t.Error("Delete should enqueue dependents")
	}

	clusterOld := &keycloakv1beta1.ClusterKeycloakRealm{Status: keycloakv1beta1.ClusterKeycloakRealmStatus{Ready: false}}
	clusterNew := &keycloakv1beta1.ClusterKeycloakRealm{Status: keycloakv1beta1.ClusterKeycloakRealmStatus{Ready: true, RealmName: "r"}}
	if !p.Update(event.UpdateEvent{ObjectOld: clusterOld, ObjectNew: clusterNew}) {
		t.Error("cluster realm readiness flip should enqueue dependents")
	}
}
