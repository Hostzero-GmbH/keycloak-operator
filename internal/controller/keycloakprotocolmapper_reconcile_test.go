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

const (
	fakeMapperScopeID = "scope-id"
	fakeMapperID      = "mapper-id"
)

// fakeProtocolMapperKeycloak is a minimal stateful stand-in for the protocol
// mapper endpoints of one client scope. As on Keycloak 26.8, a PUT replaces the
// whole config, and a config that carries id.token.claim or access.token.claim
// gets the introspection.token.claim and userinfo.token.claim defaults added on
// create and on every PUT. Writes are counted so tests can assert that a steady
// state stays silent.
type fakeProtocolMapperKeycloak struct {
	t      *testing.T
	mapper map[string]interface{}
	posts  int
	puts   int
}

func (f *fakeProtocolMapperKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	handleToken(mux)

	const mappers = "/admin/realms/test/client-scopes/" + fakeMapperScopeID + "/protocol-mappers/models"
	mux.HandleFunc(mappers, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.mapper == nil {
				writeUserJSON(w, []interface{}{})
				return
			}
			writeUserJSON(w, []interface{}{f.mapper})
		case http.MethodPost:
			f.posts++
			f.store(decodeObject(f.t, r))
			w.Header().Set("Location", mappers+"/"+fakeMapperID)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(mappers+"/"+fakeMapperID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f.puts++
		f.store(decodeObject(f.t, r))
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (f *fakeProtocolMapperKeycloak) store(mapper map[string]interface{}) {
	mapper["id"] = fakeMapperID
	mapper["consentRequired"] = false
	if config, ok := mapper["config"].(map[string]interface{}); ok {
		_, hasIDToken := config["id.token.claim"]
		_, hasAccessToken := config["access.token.claim"]
		if hasIDToken || hasAccessToken {
			config["introspection.token.claim"] = "true"
			config["userinfo.token.claim"] = "true"
		}
	}
	f.mapper = mapper
}

func newProtocolMapperFixture(claimName string) *keycloakv1beta1.KeycloakProtocolMapper {
	return &keycloakv1beta1.KeycloakProtocolMapper{
		ObjectMeta: withFinalizer(metav1.ObjectMeta{Name: "group-membership-mapper"}),
		Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
			ClientScopeRef: &keycloakv1beta1.ResourceRef{Name: "groups-scope"},
			Name:           strPtr("groups"),
			Definition:     protocolMapperDefinition(claimName),
		},
	}
}

func protocolMapperDefinition(claimName string) runtime.RawExtension {
	return runtime.RawExtension{Raw: []byte(`{
		"protocol": "openid-connect",
		"protocolMapper": "oidc-group-membership-mapper",
		"config": {
			"claim.name": "` + claimName + `",
			"full.path": "false",
			"id.token.claim": "true",
			"access.token.claim": "true"
		}
	}`)}
}

// readyParentScope is the KeycloakClientScope the mapper hangs off. The mapper
// controller only reads its status, so the scope itself does not have to exist
// on the fake.
func readyParentScope() *keycloakv1beta1.KeycloakClientScope {
	scope := newClientScopeFixture(`{"protocol":"openid-connect"}`)
	scope.Status = keycloakv1beta1.KeycloakClientScopeStatus{
		Ready:        true,
		ResourcePath: "/admin/realms/test/client-scopes/" + fakeMapperScopeID,
	}
	return scope
}

func TestKeycloakProtocolMapperReconcile_SkipsUnchangedPut(t *testing.T) {
	fakeKC := &fakeProtocolMapperKeycloak{t: t}
	mapper := newProtocolMapperFixture("groups")
	h := newReconcileHarness(t, fakeKC.handler(), readyParentScope(), mapper)
	r := &KeycloakProtocolMapperReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, mapper.Name)
	require.Equal(t, 1, fakeKC.posts, "the first reconcile creates the mapper")
	require.Contains(t, fakeKC.mapper["config"], "userinfo.token.claim", "Keycloak adds default config keys of its own")

	for range 4 {
		h.reconcile(t, r, mapper.Name)
	}
	require.Equal(t, 0, fakeKC.puts,
		"config keys added by Keycloak are not drift: a mapper that already matches must not be written again")

	got := &keycloakv1beta1.KeycloakProtocolMapper{}
	require.NoError(t, h.client.Get(context.Background(), client.ObjectKeyFromObject(mapper), got))
	require.True(t, got.Status.Ready)
	require.Equal(t, fakeMapperID, got.Status.MapperID)
}

func TestKeycloakProtocolMapperReconcile_PutsOnChange(t *testing.T) {
	fakeKC := &fakeProtocolMapperKeycloak{t: t}
	mapper := newProtocolMapperFixture("groups")
	h := newReconcileHarness(t, fakeKC.handler(), readyParentScope(), mapper)
	r := &KeycloakProtocolMapperReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, mapper.Name)
	h.reconcile(t, r, mapper.Name)
	require.Equal(t, 0, fakeKC.puts)

	updateObject(t, h, mapper, func(m *keycloakv1beta1.KeycloakProtocolMapper) {
		m.Spec.Definition = protocolMapperDefinition("roles")
	})
	h.reconcile(t, r, mapper.Name)
	require.Equal(t, 1, fakeKC.puts, "a changed config value must be written")
	require.Equal(t, "roles", fakeKC.mapper["config"].(map[string]interface{})["claim.name"])

	h.reconcile(t, r, mapper.Name)
	require.Equal(t, 1, fakeKC.puts, "and only once")
}
