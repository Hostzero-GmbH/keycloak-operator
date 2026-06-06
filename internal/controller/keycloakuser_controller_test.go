package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr/testr"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// fakeUserKeycloak simulates Keycloak admin API for user role/group endpoints.
type fakeUserKeycloak struct {
	t            *testing.T
	realmRoles   []keycloak.RoleRepresentation
	userRealm    []keycloak.RoleRepresentation
	clientRoles  map[string][]keycloak.RoleRepresentation
	userClient   map[string][]keycloak.RoleRepresentation
	groups       []keycloak.GroupRepresentation
	userGroups   []keycloak.GroupRepresentation
	addRealmCall int
	delRealmCall int
	addClient    map[string]int
	delClient    map[string]int
	addGroup     map[string]int
	delGroup     map[string]int
}

func newFakeUserKeycloak(t *testing.T) *fakeUserKeycloak {
	return &fakeUserKeycloak{
		t:           t,
		clientRoles: make(map[string][]keycloak.RoleRepresentation),
		userClient:  make(map[string][]keycloak.RoleRepresentation),
		addClient:   make(map[string]int),
		delClient:   make(map[string]int),
		addGroup:    make(map[string]int),
		delGroup:    make(map[string]int),
	}
}

func (f *fakeUserKeycloak) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})

	// GET /admin/realms/{realm}/roles — list all realm roles
	mux.HandleFunc("/admin/realms/test/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeUserJSON(w, f.realmRoles)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	// GET /admin/realms/{realm}/users/{id}/role-mappings/realm — user realm role mappings
	// POST/DELETE — add/remove realm role mappings
	mux.HandleFunc("/admin/realms/test/users/user-123/role-mappings/realm", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeUserJSON(w, f.userRealm)
		case http.MethodPost:
			f.addRealmCall++
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			f.delRealmCall++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// GET /admin/realms/{realm}/clients — find client by clientId
	mux.HandleFunc("/admin/realms/test/clients", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		clientID := r.URL.Query().Get("clientId")
		if clientID == "my-app" {
			writeUserJSON(w, []keycloak.ClientRepresentation{
				{ID: strPtr("client-uuid-123")},
			})
			return
		}
		writeUserJSON(w, []keycloak.ClientRepresentation{})
	})

	// GET /admin/realms/{realm}/clients/{uuid}/roles — list client roles
	mux.HandleFunc("/admin/realms/test/clients/client-uuid-123/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeUserJSON(w, f.clientRoles["client-uuid-123"])
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	// GET /admin/realms/{realm}/users/{id}/role-mappings — composite endpoint
	mux.HandleFunc("/admin/realms/test/users/user-123/role-mappings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return composite with clientMappings populated from userClient data
			cm := make(map[string]keycloak.ClientRoleMappingsEntry)
			for uuid, roles := range f.userClient {
				if len(roles) > 0 {
					cm[uuid] = keycloak.ClientRoleMappingsEntry{
						ID:       uuid,
						Client:   uuid,
						Mappings: roles,
					}
				}
			}
			writeUserJSON(w, keycloak.UserRoleMappingsComposite{
				RealmMappings:  f.userRealm,
				ClientMappings: cm,
			})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// GET/POST/DELETE /admin/realms/{realm}/users/{id}/role-mappings/clients/{uuid}
	mux.HandleFunc("/admin/realms/test/users/user-123/role-mappings/clients/client-uuid-123", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeUserJSON(w, f.userClient["client-uuid-123"])
		case http.MethodPost:
			clientID := "client-uuid-123"
			f.addClient[clientID]++
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			clientID := "client-uuid-123"
			f.delClient[clientID]++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// GET /admin/realms/{realm}/groups — list all groups
	mux.HandleFunc("/admin/realms/test/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeUserJSON(w, f.groups)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	// GET /admin/realms/{realm}/users/{id}/groups — list user's groups
	// PUT /admin/realms/{realm}/users/{id}/groups/{groupId} — add to group
	// DELETE /admin/realms/{realm}/users/{id}/groups/{groupId} — remove from group
	mux.HandleFunc("/admin/realms/test/users/user-123/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeUserJSON(w, f.userGroups)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	// Catch PUT/DELETE to /admin/realms/test/users/user-123/groups/{groupId}
	mux.HandleFunc("/admin/realms/test/users/user-123/groups/", func(w http.ResponseWriter, r *http.Request) {
		rest := r.URL.Path[len("/admin/realms/test/users/user-123/groups/"):]
		switch r.Method {
		case http.MethodPut:
			f.addGroup[rest]++
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			f.delGroup[rest]++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return mux
}

func (f *fakeUserKeycloak) client(baseURL string) *keycloak.Client {
	return keycloak.NewClient(keycloak.Config{
		BaseURL:  baseURL,
		Realm:    "master",
		ClientID: "admin-cli",
	}, testr.New(f.t))
}

func writeUserJSON(w http.ResponseWriter, v interface{}) {
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

func TestReconcileUserRealmRoles_Assign(t *testing.T) {
	fake := newFakeUserKeycloak(t)
	fake.realmRoles = []keycloak.RoleRepresentation{
		{ID: strPtr("role-1"), Name: strPtr("offline_access")},
		{ID: strPtr("role-2"), Name: strPtr("uma_authorization")},
	}
	// user has no realm roles yet
	fake.userRealm = []keycloak.RoleRepresentation{}

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	r := &KeycloakUserReconciler{}
	err := r.reconcileUserRealmRoles(context.Background(), kc, "test", "user-123", []string{"offline_access"})
	if err != nil {
		t.Fatalf("reconcileUserRealmRoles: %v", err)
	}
	if fake.addRealmCall != 1 {
		t.Errorf("add realm roles call count = %d, want 1", fake.addRealmCall)
	}
	if fake.delRealmCall != 0 {
		t.Errorf("del realm roles call count = %d, want 0", fake.delRealmCall)
	}
}

func TestReconcileUserRealmRoles_Remove(t *testing.T) {
	fake := newFakeUserKeycloak(t)
	fake.realmRoles = []keycloak.RoleRepresentation{
		{ID: strPtr("role-1"), Name: strPtr("offline_access")},
		{ID: strPtr("role-2"), Name: strPtr("uma_authorization")},
	}
	// user has uma_authorization but doesn't want it
	fake.userRealm = []keycloak.RoleRepresentation{
		{ID: strPtr("role-2"), Name: strPtr("uma_authorization")},
	}

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	r := &KeycloakUserReconciler{}
	err := r.reconcileUserRealmRoles(context.Background(), kc, "test", "user-123", []string{"offline_access"})
	if err != nil {
		t.Fatalf("reconcileUserRealmRoles: %v", err)
	}
	if fake.addRealmCall != 1 {
		t.Errorf("add realm roles call count = %d, want 1", fake.addRealmCall)
	}
	if fake.delRealmCall != 1 {
		t.Errorf("del realm roles call count = %d, want 1", fake.delRealmCall)
	}
}

func TestReconcileUserClientRoles_Partial(t *testing.T) {
	fake := newFakeUserKeycloak(t)
	fake.clientRoles["client-uuid-123"] = []keycloak.RoleRepresentation{
		{ID: strPtr("cr-1"), Name: strPtr("admin")},
		{ID: strPtr("cr-2"), Name: strPtr("viewer")},
		{ID: strPtr("cr-3"), Name: strPtr("editor")},
	}
	// user already has admin + viewer
	fake.userClient["client-uuid-123"] = []keycloak.RoleRepresentation{
		{ID: strPtr("cr-1"), Name: strPtr("admin")},
		{ID: strPtr("cr-2"), Name: strPtr("viewer")},
	}

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	r := &KeycloakUserReconciler{}
	err := r.reconcileUserClientRoles(context.Background(), kc, "test", "user-123", map[string][]string{
		"my-app": {"admin", "editor"},
	})
	if err != nil {
		t.Fatalf("reconcileUserClientRoles: %v", err)
	}
	// Should add editor, remove viewer
	if fake.addClient["client-uuid-123"] != 1 {
		t.Errorf("add client roles call count = %d, want 1", fake.addClient["client-uuid-123"])
	}
	if fake.delClient["client-uuid-123"] != 1 {
		t.Errorf("del client roles call count = %d, want 1", fake.delClient["client-uuid-123"])
	}
}

func TestReconcileUserGroups_JoinAndLeave(t *testing.T) {
	fake := newFakeUserKeycloak(t)
	fake.groups = []keycloak.GroupRepresentation{
		{ID: strPtr("g-1"), Name: strPtr("admins")},
		{ID: strPtr("g-2"), Name: strPtr("developers")},
		{ID: strPtr("g-3"), Name: strPtr("viewers")},
	}
	// user is already in admins and viewers
	fake.userGroups = []keycloak.GroupRepresentation{
		{ID: strPtr("g-1"), Name: strPtr("admins")},
		{ID: strPtr("g-3"), Name: strPtr("viewers")},
	}

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	r := &KeycloakUserReconciler{}
	err := r.reconcileUserGroups(context.Background(), kc, "test", "user-123", []string{"admins", "developers"})
	if err != nil {
		t.Fatalf("reconcileUserGroups: %v", err)
	}
	// Should join developers (g-2), leave viewers (g-3)
	if fake.addGroup["g-2"] != 1 {
		t.Errorf("add group 'developers' call count = %d, want 1", fake.addGroup["g-2"])
	}
}

func TestReconcileUserRealmRoles_Noop(t *testing.T) {
	fake := newFakeUserKeycloak(t)
	// empty input — should return immediately with no calls
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	r := &KeycloakUserReconciler{}
	err := r.reconcileUserRealmRoles(context.Background(), kc, "test", "user-123", nil)
	if err != nil {
		t.Fatalf("reconcileUserRealmRoles: %v", err)
	}
	if fake.addRealmCall != 0 {
		t.Errorf("expected no API calls on empty input")
	}
}
