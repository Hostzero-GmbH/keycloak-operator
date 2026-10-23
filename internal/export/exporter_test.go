package export

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"

	"github.com/go-logr/logr/testr"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// fakeKeycloak is a minimal Keycloak admin API stand-in used to exercise the
// exporter against both the legacy (pre-23) inline subGroups response shape
// and the modern (23+) shape that requires fetching children separately.
type fakeKeycloak struct {
	t *testing.T

	// topLevel is the response body for GET /groups for the test realm.
	topLevel []json.RawMessage

	// children maps a parent group ID to the full children list returned by
	// GET /groups/{id}/children. The fake paginates the slice using the
	// `first` and `max` query parameters supplied by the client.
	children map[string][]json.RawMessage

	// realmRoles maps a group ID to the role mappings returned by
	// GET /groups/{id}/role-mappings/realm.
	realmRoles map[string][]json.RawMessage

	// childrenCalls counts requests per group ID for assertions about
	// pagination and the post-23 fallback.
	childrenCalls map[string]int

	// components is the response body for GET /components for the test realm.
	components []json.RawMessage
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	return &fakeKeycloak{
		t:             t,
		children:      map[string][]json.RawMessage{},
		realmRoles:    map[string][]json.RawMessage{},
		childrenCalls: map[string]int{},
	}
}

func (f *fakeKeycloak) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})

	mux.HandleFunc("/admin/realms/test/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/realms/test/groups" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, f.topLevel)
	})

	mux.HandleFunc("/admin/realms/test/groups/", func(w http.ResponseWriter, r *http.Request) {
		// Path forms we care about:
		//   /admin/realms/test/groups/{id}/children
		//   /admin/realms/test/groups/{id}/role-mappings/realm
		rest := r.URL.Path[len("/admin/realms/test/groups/"):]
		switch {
		case len(rest) > len("/children") && rest[len(rest)-len("/children"):] == "/children":
			id := rest[:len(rest)-len("/children")]
			f.childrenCalls[id]++
			page := paginate(f.children[id], r.URL.Query())
			writeJSON(w, page)
		case len(rest) > len("/role-mappings/realm") && rest[len(rest)-len("/role-mappings/realm"):] == "/role-mappings/realm":
			id := rest[:len(rest)-len("/role-mappings/realm")]
			writeJSON(w, f.realmRoles[id])
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/admin/realms/test/components", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, f.components)
	})

	mux.HandleFunc("/admin/realms/test/clients", func(w http.ResponseWriter, _ *http.Request) {
		// Returning an empty client list is enough: exportGroupRoleMappings
		// only iterates clients to fetch client-role mappings.
		writeJSON(w, []json.RawMessage{})
	})

	return mux
}

func paginate(items []json.RawMessage, q map[string][]string) []json.RawMessage {
	first := 0
	if v := q["first"]; len(v) > 0 {
		if n, err := strconv.Atoi(v[0]); err == nil {
			first = n
		}
	}
	max := len(items)
	if v := q["max"]; len(v) > 0 {
		if n, err := strconv.Atoi(v[0]); err == nil && n >= 0 {
			max = n
		}
	}
	if first >= len(items) {
		return []json.RawMessage{}
	}
	end := first + max
	if end > len(items) {
		end = len(items)
	}
	return items[first:end]
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if body == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}
	_, _ = w.Write(body)
}

func newTestExporter(t *testing.T, fake *fakeKeycloak) (*Exporter, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	client := keycloak.NewClient(keycloak.Config{
		BaseURL:      srv.URL,
		Realm:        "master",
		ClientID:     "admin-cli",
		ClientSecret: "secret",
	}, testr.New(t))

	exp := NewExporter(client, testr.New(t), ExporterOptions{
		Realm:           "test",
		TargetNamespace: "default",
	})
	return exp, srv
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func collectGroupNames(t *testing.T, resources []ExportedResource) map[string]*keycloakv1beta1.KeycloakGroup {
	t.Helper()
	groups := map[string]*keycloakv1beta1.KeycloakGroup{}
	for _, r := range resources {
		if r.Kind != "KeycloakGroup" {
			continue
		}
		g, ok := r.Object.(*keycloakv1beta1.KeycloakGroup)
		if !ok {
			t.Fatalf("expected *KeycloakGroup, got %T", r.Object)
		}
		groups[g.Name] = g
	}
	return groups
}

// TestExportGroups_LegacyInlineSubGroups verifies the pre-23 behavior where
// the realm-wide /groups response inlines the nested subGroups slice. The
// exporter must not fetch /children at all in that case.
func TestExportGroups_LegacyInlineSubGroups(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.topLevel = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":   "parent-id",
			"name": "parent",
			"path": "/parent",
			"subGroups": []map[string]interface{}{{
				"id":        "child-id",
				"name":      "child",
				"path":      "/parent/child",
				"subGroups": []interface{}{},
			}},
		}),
	}

	exp, _ := newTestExporter(t, fake)
	resources, err := exp.exportGroups(context.Background())
	if err != nil {
		t.Fatalf("exportGroups: %v", err)
	}

	groups := collectGroupNames(t, resources)
	if _, ok := groups["parent"]; !ok {
		t.Fatalf("missing top-level group 'parent', got %v", keys(groups))
	}
	if _, ok := groups["parent-child"]; !ok {
		t.Fatalf("missing child group 'parent-child', got %v", keys(groups))
	}

	if calls := fake.childrenCalls["parent-id"]; calls != 0 {
		t.Errorf("did not expect /children calls when subGroups is inline, got %d", calls)
	}
}

// TestExportGroups_Keycloak23ChildrenFallback verifies the post-23 behavior:
// /groups returns subGroupCount > 0 with an empty subGroups array, so the
// exporter must fall back to /groups/{id}/children to find subgroups, and
// recurse into them when they themselves have children.
func TestExportGroups_Keycloak23ChildrenFallback(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.topLevel = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "parent-id",
			"name":          "parent",
			"path":          "/parent",
			"subGroupCount": 2,
			"subGroups":     []interface{}{},
		}),
	}
	fake.children["parent-id"] = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "child-a-id",
			"name":          "child-a",
			"path":          "/parent/child-a",
			"subGroupCount": 1,
			"subGroups":     []interface{}{},
		}),
		mustJSON(t, map[string]interface{}{
			"id":            "child-b-id",
			"name":          "child-b",
			"path":          "/parent/child-b",
			"subGroupCount": 0,
			"subGroups":     []interface{}{},
		}),
	}
	fake.children["child-a-id"] = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "grandchild-id",
			"name":          "grand",
			"path":          "/parent/child-a/grand",
			"subGroupCount": 0,
			"subGroups":     []interface{}{},
		}),
	}

	exp, _ := newTestExporter(t, fake)
	resources, err := exp.exportGroups(context.Background())
	if err != nil {
		t.Fatalf("exportGroups: %v", err)
	}

	groups := collectGroupNames(t, resources)
	wantNames := []string{"parent", "parent-child-a", "parent-child-b", "parent-child-a-grand"}
	for _, name := range wantNames {
		if _, ok := groups[name]; !ok {
			t.Fatalf("missing exported group %q, got %v", name, keys(groups))
		}
	}

	// Verify parent references on subgroups.
	if got := groups["parent-child-a"].Spec.ParentGroupRef; got == nil || got.Name != "parent" {
		t.Errorf("child-a parentGroupRef = %+v, want parent", got)
	}
	if got := groups["parent-child-b"].Spec.ParentGroupRef; got == nil || got.Name != "parent" {
		t.Errorf("child-b parentGroupRef = %+v, want parent", got)
	}
	if got := groups["parent-child-a-grand"].Spec.ParentGroupRef; got == nil || got.Name != "parent-child-a" {
		t.Errorf("grandchild parentGroupRef = %+v, want parent-child-a", got)
	}

	if calls := fake.childrenCalls["parent-id"]; calls < 1 {
		t.Errorf("expected at least 1 /children call for parent, got %d", calls)
	}
	if calls := fake.childrenCalls["child-a-id"]; calls < 1 {
		t.Errorf("expected at least 1 /children call for child-a (it has subGroupCount=1), got %d", calls)
	}
	if calls := fake.childrenCalls["child-b-id"]; calls != 0 {
		t.Errorf("did not expect /children call for child-b (subGroupCount=0), got %d", calls)
	}
}

// TestExportGroups_ChildrenPagination verifies that the exporter pages through
// /children when a parent has more children than the page size.
func TestExportGroups_ChildrenPagination(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.topLevel = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "parent-id",
			"name":          "parent",
			"path":          "/parent",
			"subGroupCount": groupChildrenPageSize + 1,
			"subGroups":     []interface{}{},
		}),
	}
	for i := 0; i < groupChildrenPageSize+1; i++ {
		fake.children["parent-id"] = append(fake.children["parent-id"], mustJSON(t, map[string]interface{}{
			"id":            fmt.Sprintf("child-%d-id", i),
			"name":          fmt.Sprintf("child-%d", i),
			"path":          fmt.Sprintf("/parent/child-%d", i),
			"subGroupCount": 0,
			"subGroups":     []interface{}{},
		}))
	}

	exp, _ := newTestExporter(t, fake)
	resources, err := exp.exportGroups(context.Background())
	if err != nil {
		t.Fatalf("exportGroups: %v", err)
	}

	groups := collectGroupNames(t, resources)
	if got, want := len(groups), groupChildrenPageSize+2; got != want { // +1 parent
		t.Fatalf("group count = %d, want %d (parent + %d children)", got, want, groupChildrenPageSize+1)
	}
	if calls := fake.childrenCalls["parent-id"]; calls != 2 {
		t.Errorf("expected 2 paginated /children calls, got %d", calls)
	}
}

// TestExportGroups_SubGroupRoleMappings verifies that role mappings are now
// exported for nested subgroups, not only for top-level groups.
func TestExportGroups_SubGroupRoleMappings(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.topLevel = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "parent-id",
			"name":          "parent",
			"path":          "/parent",
			"subGroupCount": 1,
			"subGroups":     []interface{}{},
		}),
	}
	fake.children["parent-id"] = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":            "child-id",
			"name":          "child",
			"path":          "/parent/child",
			"subGroupCount": 0,
			"subGroups":     []interface{}{},
		}),
	}
	fake.realmRoles["child-id"] = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id":   "role-id",
			"name": "viewer",
		}),
	}

	exp, _ := newTestExporter(t, fake)
	resources, err := exp.exportGroups(context.Background())
	if err != nil {
		t.Fatalf("exportGroups: %v", err)
	}

	var foundMapping bool
	for _, r := range resources {
		if r.Kind == "KeycloakRoleMapping" {
			foundMapping = true
		}
	}
	if !foundMapping {
		t.Fatalf("expected at least one KeycloakRoleMapping for subgroup, got resources: %v", resourceKinds(resources))
	}
}

func keys(m map[string]*keycloakv1beta1.KeycloakGroup) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func resourceKinds(rs []ExportedResource) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Kind+"/"+r.Name)
	}
	return out
}

// Components are listed without a parent filter, so LDAP mappers come back
// next to their provider. Mappers get parentComponentRef with the provider's
// CR name as prefix; a mapper whose parent is filtered out is skipped (#167).
func TestExportComponents_SubComponentsReferenceParent(t *testing.T) {
	fake := newFakeKeycloak(t)
	// Child listed before its parent on purpose: order must not matter.
	fake.components = []json.RawMessage{
		mustJSON(t, map[string]interface{}{
			"id": "mapper-1", "parentId": "ldap-1", "name": "username",
			"providerId": "user-attribute-ldap-mapper", "providerType": "org.keycloak.storage.ldap.mappers.LDAPStorageMapper",
		}),
		mustJSON(t, map[string]interface{}{
			"id": "ldap-1", "parentId": "realm-id", "name": "corporate-ldap",
			"providerId": "ldap", "providerType": "org.keycloak.storage.UserStorageProvider",
		}),
		mustJSON(t, map[string]interface{}{
			"id": "key-1", "parentId": "realm-id", "name": "rsa-generated",
			"providerId": "rsa-generated", "providerType": "org.keycloak.keys.KeyProvider",
		}),
		mustJSON(t, map[string]interface{}{
			"id": "orphan-1", "parentId": "key-1", "name": "under-filtered-parent",
			"providerId": "whatever", "providerType": "org.keycloak.storage.ldap.mappers.LDAPStorageMapper",
		}),
	}
	exp, _ := newTestExporter(t, fake)
	exp.filter = NewFilter(nil, nil, true) // --skip-defaults drops the key provider

	resources, err := exp.exportComponents(context.Background())
	if err != nil {
		t.Fatalf("exportComponents: %v", err)
	}

	byName := map[string]*keycloakv1beta1.KeycloakComponent{}
	for _, r := range resources {
		c, ok := r.Object.(*keycloakv1beta1.KeycloakComponent)
		if !ok {
			t.Fatalf("expected *KeycloakComponent, got %T", r.Object)
		}
		byName[r.Name] = c
	}
	if len(byName) != 2 {
		t.Fatalf("expected the provider and its mapper only, got %d: %v", len(byName), byName)
	}

	provider := byName["userstorageprovider-corporate-ldap"]
	if provider == nil || provider.Spec.RealmRef == nil || provider.Spec.ParentComponentRef != nil {
		t.Fatalf("top-level provider must keep realmRef and have no parentComponentRef: %+v", provider)
	}

	mapper := byName["userstorageprovider-corporate-ldap-ldapstoragemapper-username"]
	if mapper == nil {
		t.Fatalf("mapper not exported under the parent-prefixed name; got %v", byName)
	}
	if mapper.Spec.RealmRef != nil {
		t.Errorf("sub-component must not carry realmRef")
	}
	if mapper.Spec.ParentComponentRef == nil || mapper.Spec.ParentComponentRef.Name != "userstorageprovider-corporate-ldap" {
		t.Errorf("parentComponentRef = %+v, want the provider's CR name", mapper.Spec.ParentComponentRef)
	}
}
