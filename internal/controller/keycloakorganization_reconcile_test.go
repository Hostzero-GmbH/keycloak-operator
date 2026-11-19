package controller

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

const fakeOrganizationID = "org-id"

var fakeOrganizationKeys = map[string]bool{
	"id": true, "name": true, "alias": true, "enabled": true, "description": true,
	"redirectUrl": true, "attributes": true, "domains": true,
	"identityProviders": true, "members": true, "groups": true,
}

// fakeOrganizationKeycloak is a minimal stateful stand-in for the organization
// admin endpoints. It follows what Keycloak 26.7 does: a request with a key the
// representation does not have is answered with 400, the id of a created
// organization is generated, and identityProviders, members and groups are
// accepted and discarded. A PUT replaces description, redirectUrl and domains
// with what it carries, sets enabled back to true when it has none, and leaves
// attributes alone when it has none. Writes are counted so tests can assert that
// a steady state stays silent.
type fakeOrganizationKeycloak struct {
	t        *testing.T
	org      map[string]interface{}
	posts    int
	puts     int
	lastPost map[string]interface{}
	lastPut  map[string]interface{}
	// failGet makes GET /organizations/{id} answer 500.
	failGet bool
}

func (f *fakeOrganizationKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	handleToken(mux)

	const organizations = "/admin/realms/test/organizations"
	mux.HandleFunc(organizations, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.org == nil {
				writeUserJSON(w, []interface{}{})
				return
			}
			writeUserJSON(w, []interface{}{map[string]interface{}{
				"id": fakeOrganizationID, "name": f.org["name"], "alias": f.org["alias"], "enabled": f.org["enabled"],
			}})
		case http.MethodPost:
			body := decodeObject(f.t, r)
			if !rejectUnknownOrganizationKeys(w, body) {
				return
			}
			f.posts++
			f.lastPost = body
			f.org = map[string]interface{}{"id": fakeOrganizationID}
			f.store(body)
			w.Header().Set("Location", organizations+"/"+fakeOrganizationID)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(organizations+"/"+fakeOrganizationID, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if f.failGet {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			writeUserJSON(w, f.org)
		case http.MethodPut:
			body := decodeObject(f.t, r)
			if !rejectUnknownOrganizationKeys(w, body) {
				return
			}
			f.puts++
			f.lastPut = body
			f.store(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func rejectUnknownOrganizationKeys(w http.ResponseWriter, body map[string]interface{}) bool {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !fakeOrganizationKeys[key] {
			http.Error(w, fmt.Sprintf(`{"error":"Invalid json representation for OrganizationRepresentation. Unrecognized field \"%s\""}`, key), http.StatusBadRequest)
			return false
		}
	}
	return true
}

func (f *fakeOrganizationKeycloak) store(body map[string]interface{}) {
	for _, key := range []string{"name", "alias", "attributes"} {
		if value, ok := body[key]; ok {
			f.org[key] = value
		}
	}
	if value, ok := body["enabled"]; ok {
		f.org["enabled"] = value
	} else {
		f.org["enabled"] = true
	}
	// Keycloak keeps an empty description as it is, and stores an emptied
	// redirectUrl or domains list as no value, which it then leaves out.
	for _, key := range []string{"description", "redirectUrl", "domains"} {
		value, ok := body[key]
		switch {
		case !ok || value == nil:
			delete(f.org, key)
		case key != "description" && isEmptyDefinitionValue(value):
			delete(f.org, key)
		default:
			f.org[key] = value
		}
	}
	if domains, ok := f.org["domains"].([]interface{}); ok {
		for _, domain := range domains {
			if entry, ok := domain.(map[string]interface{}); ok {
				if _, set := entry["verified"]; !set {
					entry["verified"] = false
				}
			}
		}
	}
}

func newOrganizationFixture(definition string) *keycloakv1beta1.KeycloakOrganization {
	return &keycloakv1beta1.KeycloakOrganization{
		ObjectMeta: withFinalizer(metav1.ObjectMeta{Name: "acme-org"}),
		Spec: keycloakv1beta1.KeycloakOrganizationSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: "realm"},
			Name:       strPtr("acme"),
			Definition: runtime.RawExtension{Raw: []byte(definition)},
		},
	}
}

func setOrganizationDefinition(definition string) func(*keycloakv1beta1.KeycloakOrganization) {
	return func(o *keycloakv1beta1.KeycloakOrganization) {
		o.Spec.Definition = runtime.RawExtension{Raw: []byte(definition)}
	}
}

func getOrganization(t *testing.T, h *reconcileHarness, org *keycloakv1beta1.KeycloakOrganization) *keycloakv1beta1.KeycloakOrganization {
	t.Helper()
	got := &keycloakv1beta1.KeycloakOrganization{}
	require.NoError(t, h.client.Get(context.Background(), client.ObjectKeyFromObject(org), got))
	return got
}

func TestKeycloakOrganizationReconcile_SendsEveryDeclaredField(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","enabled":true,"description":"d","redirectUrl":"https://app.example.com/after","attributes":{"tier":["gold"]},"domains":[{"name":"acme.com"}]}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, org.Name)

	require.Equal(t, 1, fakeKC.posts, "the first reconcile creates the organization")
	require.Equal(t, "acme", fakeKC.lastPost["name"], "the name comes from spec.name")
	require.Equal(t, "https://app.example.com/after", fakeKC.org["redirectUrl"], "redirectUrl has to reach Keycloak")
	require.Equal(t, "d", fakeKC.org["description"])

	got := getOrganization(t, h, org)
	require.True(t, got.Status.Ready)
	require.Equal(t, fakeOrganizationID, got.Status.OrganizationID)
	require.Equal(t, "acme", got.Status.OrganizationName)
	require.Equal(t, "Organization synchronized", got.Status.Message, "nothing was ignored, so nothing is named")
}

func TestKeycloakOrganizationReconcile_SkipsUnchangedPutAndNamesIgnoredKeys(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{
		"alias": "acme",
		"redirectUrl": "https://app.example.com/after",
		"id": "00000000-0000-0000-0000-000000000000",
		"identityProviders": [{"alias": "idp1"}],
		"members": [{"username": "someone"}]
	}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.posts)
	for _, key := range []string{"id", "identityProviders", "members"} {
		require.NotContains(t, fakeKC.lastPost, key, "Keycloak ignores %s, so it is not sent", key)
	}

	for range 4 {
		h.reconcile(t, r, org.Name)
	}
	require.Equal(t, 0, fakeKC.puts,
		"keys Keycloak never returns must not keep an organization out of sync")

	got := getOrganization(t, h, org)
	require.True(t, got.Status.Ready)
	require.Equal(t, "Organization synchronized; ignored definition keys: id, identityProviders, members", got.Status.Message)
}

func TestKeycloakOrganizationReconcile_PutsChangedRedirectUrlOnce(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","redirectUrl":"https://app.example.com/before"}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, org.Name)
	h.reconcile(t, r, org.Name)
	require.Equal(t, 0, fakeKC.puts)

	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","redirectUrl":"https://app.example.com/after"}`))
	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "a changed redirectUrl must be written")
	require.Equal(t, "https://app.example.com/after", fakeKC.org["redirectUrl"])

	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "and only once")
}

func TestKeycloakOrganizationReconcile_KeepsUndeclaredFieldsOnUpdate(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","description":"first"}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, org.Name)

	// Set in the Admin Console: the resource does not declare them.
	fakeKC.org["redirectUrl"] = "https://admin.example.com/console"
	fakeKC.org["enabled"] = false

	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","description":"second"}`))
	h.reconcile(t, r, org.Name)

	require.Equal(t, 1, fakeKC.puts, "the changed description must be written")
	require.Equal(t, "second", fakeKC.org["description"])
	require.Equal(t, "https://admin.example.com/console", fakeKC.org["redirectUrl"],
		"a field the resource does not declare must survive Keycloak's replace semantics")
	require.Equal(t, false, fakeKC.org["enabled"],
		"an enabled flag the resource does not declare must not be set back to true")
	require.NotContains(t, fakeKC.lastPut, "members", "the overlay carries nothing Keycloak ignores")

	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "and the organization is in sync afterwards")
}

func TestKeycloakOrganizationReconcile_ClearsADeclaredEmptyField(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","description":"d","redirectUrl":"https://app.example.com/after","domains":[{"name":"acme.com"}]}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, org.Name)
	require.Contains(t, fakeKC.org, "redirectUrl")

	// Undeclared fields are left alone, so a field is cleared by declaring it
	// empty. Keycloak then leaves the key out of its answer, which must not
	// look like drift on every resync.
	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","description":"d","redirectUrl":"","domains":[]}`))
	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "the emptied fields must be written")
	require.NotContains(t, fakeKC.org, "redirectUrl")
	require.NotContains(t, fakeKC.org, "domains")
	require.Equal(t, "d", fakeKC.org["description"], "what is still declared stays")

	for range 3 {
		h.reconcile(t, r, org.Name)
	}
	require.Equal(t, 1, fakeKC.puts, "an emptied field that Keycloak no longer returns is in sync")
}

func TestKeycloakOrganizationReconcile_PropagatesRemovedAndEmptiedAttributes(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","attributes":{"team":["a"],"tier":["1"]}}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, org.Name)
	h.reconcile(t, r, org.Name)
	require.Equal(t, 0, fakeKC.puts)

	// The PUT replaces attributes as a whole, so an attribute dropped from the
	// resource must reach Keycloak although the remaining ones match.
	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","attributes":{"team":["a"]}}`))
	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "dropping an attribute is drift")
	require.NotContains(t, fakeKC.org["attributes"], "tier")
	require.Contains(t, fakeKC.org["attributes"], "team")

	// An empty map clears them all.
	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","attributes":{}}`))
	h.reconcile(t, r, org.Name)
	require.Equal(t, 2, fakeKC.puts, "an empty attributes map clears the attributes")
	require.Empty(t, fakeKC.org["attributes"])

	for range 3 {
		h.reconcile(t, r, org.Name)
	}
	require.Equal(t, 2, fakeKC.puts, "and the organization is in sync afterwards")
}

func TestKeycloakOrganizationReconcile_RetriesInsteadOfWritingWhenCurrentStateCannotBeFetched(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","description":"first"}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}
	h.reconcile(t, r, org.Name)

	fakeKC.org["redirectUrl"] = "https://admin.example.com/console"
	fakeKC.failGet = true
	updateObject(t, h, org, setOrganizationDefinition(`{"alias":"acme","description":"second"}`))
	h.reconcile(t, r, org.Name)

	// A PUT without the current state would clear what the resource does not
	// declare, so nothing is written and the failure is on the status.
	require.Equal(t, 0, fakeKC.puts, "an update must not be sent without the current organization")
	require.Equal(t, "first", fakeKC.org["description"])
	require.Equal(t, "https://admin.example.com/console", fakeKC.org["redirectUrl"])
	got := getOrganization(t, h, org)
	require.False(t, got.Status.Ready)
	require.Equal(t, "GetFailed", got.Status.Status)
	require.Contains(t, got.Status.Message, "Failed to get organization")

	fakeKC.failGet = false
	h.reconcile(t, r, org.Name)
	require.Equal(t, 1, fakeKC.puts, "once Keycloak answers again the update goes out")
	require.Equal(t, "second", fakeKC.org["description"])
	require.Equal(t, "https://admin.example.com/console", fakeKC.org["redirectUrl"])
	require.True(t, getOrganization(t, h, org).Status.Ready)
}

func TestKeycloakOrganizationReconcile_ReportsAKeyKeycloakRejects(t *testing.T) {
	fakeKC := &fakeOrganizationKeycloak{t: t}
	org := newOrganizationFixture(`{"alias":"acme","redirectURL":"https://app.example.com/after"}`)
	h := newReconcileHarness(t, fakeKC.handler(), org)
	r := &KeycloakOrganizationReconciler{Client: h.client, Scheme: h.client.Scheme(), ClientManager: h.manager}

	h.reconcile(t, r, org.Name)

	require.Equal(t, 0, fakeKC.posts)
	require.Nil(t, fakeKC.org)
	got := getOrganization(t, h, org)
	require.False(t, got.Status.Ready)
	require.Equal(t, "CreateFailed", got.Status.Status)
	require.Contains(t, got.Status.Message, "Unrecognized field", "a misspelled key is no longer dropped silently")
	require.Contains(t, got.Status.Message, "redirectURL", "Keycloak's answer names the key and is on the status")
}
