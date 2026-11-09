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

const fakeClientScopeID = "scope-id"

// fakeClientScopeKeycloak is a minimal stateful stand-in for the client scope
// admin endpoints. As on Keycloak 26.8, a PUT merges the attributes it carries
// into the stored ones instead of replacing them. Writes are counted so tests
// can assert that a steady state stays silent.
type fakeClientScopeKeycloak struct {
	t     *testing.T
	scope map[string]interface{}
	posts int
	puts  int
}

func (f *fakeClientScopeKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	handleToken(mux)

	const scopes = "/admin/realms/test/client-scopes"
	mux.HandleFunc(scopes, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.scope == nil {
				writeUserJSON(w, []interface{}{})
				return
			}
			writeUserJSON(w, []interface{}{f.scope})
		case http.MethodPost:
			f.posts++
			f.scope = decodeObject(f.t, r)
			f.scope["id"] = fakeClientScopeID
			w.Header().Set("Location", scopes+"/"+fakeClientScopeID)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(scopes+"/"+fakeClientScopeID, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeUserJSON(w, f.scope)
		case http.MethodPut:
			f.puts++
			for k, v := range decodeObject(f.t, r) {
				if k != "attributes" {
					f.scope[k] = v
					continue
				}
				stored, _ := f.scope["attributes"].(map[string]interface{})
				if stored == nil {
					stored = map[string]interface{}{}
				}
				for name, value := range v.(map[string]interface{}) {
					stored[name] = value
				}
				f.scope["attributes"] = stored
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func newClientScopeFixture(definition string) *keycloakv1beta1.KeycloakClientScope {
	return &keycloakv1beta1.KeycloakClientScope{
		ObjectMeta: withFinalizer(metav1.ObjectMeta{Name: "groups-scope"}),
		Spec: keycloakv1beta1.KeycloakClientScopeSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: "realm"},
			Name:       strPtr("groups-scope"),
			Definition: runtime.RawExtension{Raw: []byte(definition)},
		},
	}
}

func setClientScopeDefinition(definition string) func(*keycloakv1beta1.KeycloakClientScope) {
	return func(s *keycloakv1beta1.KeycloakClientScope) {
		s.Spec.Definition = runtime.RawExtension{Raw: []byte(definition)}
	}
}

func TestKeycloakClientScopeReconcile_SkipsUnchangedPut(t *testing.T) {
	fakeKC := &fakeClientScopeKeycloak{t: t}
	scope := newClientScopeFixture(`{"protocol":"openid-connect","attributes":{"include.in.token.scope":"true"}}`)
	h := newReconcileHarness(t, fakeKC.handler(), scope)
	r := &KeycloakClientScopeReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, scope.Name)
	require.Equal(t, 1, fakeKC.posts, "the first reconcile creates the client scope")

	for range 3 {
		h.reconcile(t, r, scope.Name)
	}
	require.Equal(t, 0, fakeKC.puts, "a client scope that already matches must not be written again")

	got := &keycloakv1beta1.KeycloakClientScope{}
	require.NoError(t, h.client.Get(context.Background(), client.ObjectKeyFromObject(scope), got))
	require.True(t, got.Status.Ready)
}

func TestKeycloakClientScopeReconcile_PutsOnChange(t *testing.T) {
	fakeKC := &fakeClientScopeKeycloak{t: t}
	scope := newClientScopeFixture(`{"protocol":"openid-connect","attributes":{"include.in.token.scope":"true"}}`)
	h := newReconcileHarness(t, fakeKC.handler(), scope)
	r := &KeycloakClientScopeReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, scope.Name)

	updateObject(t, h, scope, setClientScopeDefinition(`{"protocol":"openid-connect","attributes":{"include.in.token.scope":"false"}}`))
	h.reconcile(t, r, scope.Name)
	require.Equal(t, 1, fakeKC.puts, "a changed attribute value must be written")
	require.Equal(t, "false", fakeKC.scope["attributes"].(map[string]interface{})["include.in.token.scope"])

	h.reconcile(t, r, scope.Name)
	require.Equal(t, 1, fakeKC.puts, "and only once")
}
