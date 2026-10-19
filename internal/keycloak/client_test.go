package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeListServer is a minimal Keycloak Admin API stand-in that paginates a
// fixed slice of items using the standard `first` and `max` query parameters.
type fakeListServer struct {
	itemCount int
	requests  []map[string]string

	// passthroughParams records all query params that were forwarded to the
	// server on each request, so tests can verify caller-supplied filters
	// (e.g. "search", "exact") are preserved across pages.
	passthroughParams []map[string]string
}

func newFakeListServer(itemCount int) *fakeListServer {
	return &fakeListServer{itemCount: itemCount}
}

func (f *fakeListServer) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})

	mux.HandleFunc("/admin/realms/test/things", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		recorded := map[string]string{}
		for k, v := range q {
			if len(v) > 0 {
				recorded[k] = v[0]
			}
		}
		f.passthroughParams = append(f.passthroughParams, recorded)

		first, _ := strconv.Atoi(q.Get("first"))
		max, _ := strconv.Atoi(q.Get("max"))
		if max <= 0 {
			max = 100
		}

		f.requests = append(f.requests, map[string]string{
			"first": q.Get("first"),
			"max":   q.Get("max"),
		})

		end := first + max
		if end > f.itemCount {
			end = f.itemCount
		}
		if first >= f.itemCount {
			end = first
		}

		items := make([]map[string]any, 0, end-first)
		for i := first; i < end; i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("item-%d", i)})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	})

	return mux
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return NewClient(Config{
		BaseURL:      baseURL,
		Realm:        "master",
		ClientID:     "admin-cli",
		ClientSecret: "secret",
	}, testr.New(t))
}

type thing struct {
	ID string `json:"id"`
}

// TestListAll_SinglePage covers the common case where the entire result set
// fits in one page. Exactly one HTTP request must be made.
func TestListAll_SinglePage(t *testing.T) {
	fake := newFakeListServer(42)
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	got, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", nil)
	require.NoError(t, err)
	assert.Len(t, got, 42)
	assert.Len(t, fake.requests, 1, "expected exactly one request when result set is smaller than a page")
	assert.Equal(t, "0", fake.requests[0]["first"])
	assert.Equal(t, "100", fake.requests[0]["max"])
}

// TestListAll_EmptyResult covers the empty result set: exactly one request,
// zero items, no panic on appending to a nil slice.
func TestListAll_EmptyResult(t *testing.T) {
	fake := newFakeListServer(0)
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	got, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Len(t, fake.requests, 1)
}

// TestListAll_MultiPage is the regression test for the bug this MR fixes.
// Without the listAll helper the client returns at most 100 items because
// that's the Keycloak Admin API default.
func TestListAll_MultiPage(t *testing.T) {
	fake := newFakeListServer(250)
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	got, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", nil)
	require.NoError(t, err)

	require.Len(t, got, 250)
	for i, item := range got {
		assert.Equal(t, fmt.Sprintf("item-%d", i), item.ID)
	}

	require.Len(t, fake.requests, 3, "expected 3 paginated requests (100 + 100 + 50)")
	assert.Equal(t, "0", fake.requests[0]["first"])
	assert.Equal(t, "100", fake.requests[1]["first"])
	assert.Equal(t, "200", fake.requests[2]["first"])
}

// TestListAll_ExactPageBoundary covers the edge case where the total number of
// items is an exact multiple of the page size. The helper has no way of
// knowing the previous page was the last one (all pages are full), so it must
// make one extra request and stop on the empty page.
func TestListAll_ExactPageBoundary(t *testing.T) {
	fake := newFakeListServer(200)
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	got, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", nil)
	require.NoError(t, err)

	assert.Len(t, got, 200)
	assert.Len(t, fake.requests, 3, "expected 3 requests at exact page boundary (100 + 100 + 0)")
}

// TestListAll_PreservesCallerParams ensures caller-supplied query parameters
// (such as `search`/`exact` used by GetGroupByName/GetUserByUsername) are
// forwarded on every paginated request.
func TestListAll_PreservesCallerParams(t *testing.T) {
	fake := newFakeListServer(150)
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	params := map[string]string{
		"search": "foo",
		"exact":  "true",
	}
	got, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", params)
	require.NoError(t, err)
	assert.Len(t, got, 150)

	require.Len(t, fake.passthroughParams, 2)
	for i, p := range fake.passthroughParams {
		assert.Equal(t, "foo", p["search"], "request %d lost the search param", i)
		assert.Equal(t, "true", p["exact"], "request %d lost the exact param", i)
	}
}

// TestListAll_ServerError surfaces upstream errors immediately instead of
// looping forever on a failing endpoint.
func TestListAll_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/master/protocol/openid-connect/token" {
			_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	_, err := listAll[thing](context.Background(), c, "/admin/realms/test/things", nil)
	require.Error(t, err)
}

// TestIdentityProviderMappers_CRUD exercises the full create/list/get/update/
// delete cycle of the IdP-mapper Admin REST API path, which is keyed by the
// parent IdP alias rather than by realm-scoped UUIDs.
func TestIdentityProviderMappers_CRUD(t *testing.T) {
	const realm = "master"
	const alias = "oidc"

	type stored struct {
		ID                     string            `json:"id,omitempty"`
		Name                   string            `json:"name,omitempty"`
		IdentityProviderAlias  string            `json:"identityProviderAlias,omitempty"`
		IdentityProviderMapper string            `json:"identityProviderMapper,omitempty"`
		Config                 map[string]string `json:"config,omitempty"`
	}

	store := map[string]stored{}
	idCounter := 0
	base := "/admin/realms/" + realm + "/identity-provider/instances/" + alias + "/mappers"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/master/protocol/openid-connect/token" {
			_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
			return
		}

		if r.URL.Path == base {
			switch r.Method {
			case http.MethodPost:
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var s stored
				require.NoError(t, json.Unmarshal(body, &s), "POST body was: %q", string(body))
				idCounter++
				s.ID = fmt.Sprintf("mid-%d", idCounter)
				store[s.ID] = s
				w.Header().Set("Location", r.URL.Path+"/"+s.ID)
				w.WriteHeader(http.StatusCreated)
				return
			case http.MethodGet:
				items := make([]stored, 0, len(store))
				for _, v := range store {
					items = append(items, v)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(items)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if len(r.URL.Path) > len(base+"/") && r.URL.Path[:len(base+"/")] == base+"/" {
			id := r.URL.Path[len(base+"/"):]
			switch r.Method {
			case http.MethodGet:
				s, ok := store[id]
				if !ok {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(s)
				return
			case http.MethodPut:
				var s stored
				require.NoError(t, json.NewDecoder(r.Body).Decode(&s))
				s.ID = id
				store[id] = s
				w.WriteHeader(http.StatusNoContent)
				return
			case http.MethodDelete:
				if _, ok := store[id]; !ok {
					http.NotFound(w, r)
					return
				}
				delete(store, id)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	createDef, err := json.Marshal(map[string]any{
		"name":                   "role-mapper",
		"identityProviderMapper": "oidc-role-idp-mapper",
		"identityProviderAlias":  alias,
		"config": map[string]string{
			"syncMode":    "FORCE",
			"claim":       "roles",
			"claim.value": "admin",
			"role":        "realm-admin",
		},
	})
	require.NoError(t, err)

	id, err := c.CreateIdentityProviderMapper(ctx, realm, alias, createDef)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	mappers, err := c.GetIdentityProviderMappers(ctx, realm, alias)
	require.NoError(t, err)
	require.Len(t, mappers, 1)
	require.NotNil(t, mappers[0].Name)
	assert.Equal(t, "role-mapper", *mappers[0].Name)

	got, err := c.GetIdentityProviderMapper(ctx, realm, alias, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.IdentityProviderMapper)
	assert.Equal(t, "oidc-role-idp-mapper", *got.IdentityProviderMapper)
	assert.Equal(t, "FORCE", got.Config["syncMode"])

	byName, err := c.GetIdentityProviderMapperByName(ctx, realm, alias, "role-mapper")
	require.NoError(t, err)
	require.NotNil(t, byName)
	require.NotNil(t, byName.ID)
	assert.Equal(t, id, *byName.ID)

	updateDef, err := json.Marshal(map[string]any{
		"id":                     id,
		"name":                   "role-mapper",
		"identityProviderMapper": "oidc-role-idp-mapper",
		"identityProviderAlias":  alias,
		"config": map[string]string{
			"syncMode":    "INHERIT",
			"claim":       "roles",
			"claim.value": "admin",
			"role":        "realm-admin",
		},
	})
	require.NoError(t, err)
	require.NoError(t, c.UpdateIdentityProviderMapper(ctx, realm, alias, id, updateDef))

	got, err = c.GetIdentityProviderMapper(ctx, realm, alias, id)
	require.NoError(t, err)
	assert.Equal(t, "INHERIT", got.Config["syncMode"])

	require.NoError(t, c.DeleteIdentityProviderMapper(ctx, realm, alias, id))

	mappers, err = c.GetIdentityProviderMappers(ctx, realm, alias)
	require.NoError(t, err)
	assert.Empty(t, mappers)

	_, err = c.GetIdentityProviderMapperByName(ctx, realm, alias, "role-mapper")
	require.Error(t, err)
}

// TestIdentityProviderMappers_PathEscaping verifies that the alias is URL-
// path-escaped, since IdP aliases may legitimately contain characters like
// dots that should not need escaping but slash characters must be encoded.
func TestIdentityProviderMappers_PathEscaping(t *testing.T) {
	var seenPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/admin/realms/my-realm/identity-provider/instances/my%2Fweird%20alias/mappers", func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]IdentityProviderMapperRepresentation{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	_, err := c.GetIdentityProviderMappers(context.Background(), "my-realm", "my/weird alias")
	require.NoError(t, err)
	assert.Equal(t, "/admin/realms/my-realm/identity-provider/instances/my%2Fweird%20alias/mappers", seenPath)
}

// TestGetGroupByPath_PathEscaping verifies that each path segment is escaped
// individually while the separators between nested groups are preserved.
func TestGetGroupByPath_PathEscaping(t *testing.T) {
	var seenPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/admin/realms/my-realm/group-by-path/", func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"g-1","name":"child group","path":"/parent/child group"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	group, err := c.GetGroupByPath(context.Background(), "my-realm", "/parent/child group")
	require.NoError(t, err)
	assert.Equal(t, "/admin/realms/my-realm/group-by-path/parent/child%20group", seenPath)
	require.NotNil(t, group.ID)
	assert.Equal(t, "g-1", *group.ID)
}

// TestGetClients_Paginated wires the production GetClients helper end to end
// against the fake server to prove the public API actually returns more than
// one Keycloak page worth of results.
func TestGetClients_Paginated(t *testing.T) {
	const total = 213
	fake := &fakeListServer{itemCount: total}
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/admin/realms/test/clients", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		first, _ := strconv.Atoi(q.Get("first"))
		max, _ := strconv.Atoi(q.Get("max"))
		if max <= 0 {
			max = 100
		}
		end := first + max
		if end > fake.itemCount {
			end = fake.itemCount
		}
		if first >= fake.itemCount {
			end = first
		}
		items := make([]ClientRepresentation, 0, end-first)
		for i := first; i < end; i++ {
			id := fmt.Sprintf("client-%d", i)
			items = append(items, ClientRepresentation{ID: &id, ClientID: &id})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(t, srv.URL)
	clients, err := c.GetClients(context.Background(), "test", nil)
	require.NoError(t, err)
	assert.Len(t, clients, total, "GetClients must return the full result set across pages")
}
