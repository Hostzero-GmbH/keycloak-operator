package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

const fakeGroupID = "group-id"

// fakeGroupKeycloak is a minimal stateful stand-in for the group admin
// endpoints. It follows what Keycloak 26.8 does: the listing is the brief
// representation without attributes, the single-group GET adds the roles, and a
// PUT replaces attributes when the request carries them and leaves them alone
// otherwise. Writes are counted so tests can assert that a steady state stays
// silent.
type fakeGroupKeycloak struct {
	t     *testing.T
	group map[string]interface{}
	posts int
	puts  int
	// failGet makes GET /groups/{id} answer 500.
	failGet bool
}

func (f *fakeGroupKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	handleToken(mux)

	const groups = "/admin/realms/test/groups"
	mux.HandleFunc(groups, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.group == nil {
				writeUserJSON(w, []interface{}{})
				return
			}
			writeUserJSON(w, []interface{}{map[string]interface{}{
				"id": fakeGroupID, "name": f.group["name"], "path": "/" + f.group["name"].(string),
				"subGroupCount": 0, "subGroups": []interface{}{},
			}})
		case http.MethodPost:
			f.posts++
			f.group = decodeObject(f.t, r)
			f.group["id"] = fakeGroupID
			w.Header().Set("Location", groups+"/"+fakeGroupID)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(groups+"/"+fakeGroupID, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.failGet {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			full := map[string]interface{}{"realmRoles": []interface{}{}, "clientRoles": map[string]interface{}{}}
			for k, v := range f.group {
				full[k] = v
			}
			writeUserJSON(w, full)
		case http.MethodPut:
			f.puts++
			body := decodeObject(f.t, r)
			f.group["name"] = body["name"]
			if attributes, ok := body["attributes"]; ok {
				f.group["attributes"] = attributes
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func newGroupFixture(definition string) *keycloakv1beta1.KeycloakGroup {
	return &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: withFinalizer(metav1.ObjectMeta{Name: "my-group"}),
		Spec: keycloakv1beta1.KeycloakGroupSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: "realm"},
			Name:       strPtr("my-group"),
			Definition: runtime.RawExtension{Raw: []byte(definition)},
		},
	}
}

func setGroupDefinition(definition string) func(*keycloakv1beta1.KeycloakGroup) {
	return func(g *keycloakv1beta1.KeycloakGroup) {
		g.Spec.Definition = runtime.RawExtension{Raw: []byte(definition)}
	}
}

func TestKeycloakGroupReconcile_SkipsUnchangedPut(t *testing.T) {
	fakeKC := &fakeGroupKeycloak{t: t}
	group := newGroupFixture(`{"attributes":{"team":["a"],"tier":["1"]}}`)
	h := newReconcileHarness(t, fakeKC.handler(), group)
	r := &KeycloakGroupReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.posts, "the first reconcile creates the group")

	for range 3 {
		h.reconcile(t, r, group.Name)
	}
	require.Equal(t, 0, fakeKC.puts, "a group that already matches must not be written again")

	got := &keycloakv1beta1.KeycloakGroup{}
	require.NoError(t, h.client.Get(context.Background(), client.ObjectKeyFromObject(group), got))
	require.True(t, got.Status.Ready)
	require.Equal(t, fakeGroupID, got.Status.GroupID)
}

func TestKeycloakGroupReconcile_PutsOnChange(t *testing.T) {
	fakeKC := &fakeGroupKeycloak{t: t}
	group := newGroupFixture(`{"attributes":{"team":["a"],"tier":["1"]}}`)
	h := newReconcileHarness(t, fakeKC.handler(), group)
	r := &KeycloakGroupReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, group.Name)

	updateObject(t, h, group, setGroupDefinition(`{"attributes":{"team":["b"],"tier":["1"]}}`))
	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.puts, "a changed attribute value must be written")
	require.Equal(t, []interface{}{"b"}, fakeKC.group["attributes"].(map[string]interface{})["team"])

	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.puts, "and only once")
}

func TestKeycloakGroupReconcile_PropagatesRemovedAttribute(t *testing.T) {
	fakeKC := &fakeGroupKeycloak{t: t}
	group := newGroupFixture(`{"attributes":{"team":["a"],"tier":["1"]}}`)
	h := newReconcileHarness(t, fakeKC.handler(), group)
	r := &KeycloakGroupReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, group.Name)

	updateObject(t, h, group, setGroupDefinition(`{"attributes":{"team":["a"]}}`))
	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.puts, "dropping an attribute from the CR is drift")
	require.NotContains(t, fakeKC.group["attributes"], "tier")

	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.puts)
}

func TestKeycloakGroupReconcile_PutsWhenCurrentStateCannotBeFetched(t *testing.T) {
	fakeKC := &fakeGroupKeycloak{t: t}
	group := newGroupFixture(`{"attributes":{"team":["a"]}}`)
	h := newReconcileHarness(t, fakeKC.handler(), group)
	r := &KeycloakGroupReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, group.Name)

	fakeKC.failGet = true
	h.reconcile(t, r, group.Name)
	require.Equal(t, 1, fakeKC.puts, "when the current state is unknown the group is still written")
}
