package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// fakeFlowKeycloak is a minimal stateful stand-in for the authentication flow
// admin endpoints used by reconcileChildren. Executions are keyed by the alias
// of the flow they belong to, mirroring the real flow-scoped endpoints.
type fakeFlowKeycloak struct {
	t          *testing.T
	execs      map[string][]keycloak.AuthenticationExecutionInfo
	nextID     int
	deleted    []string
	subFlowDef []map[string]interface{}
}

func newFakeFlowKeycloak(t *testing.T) *fakeFlowKeycloak {
	return &fakeFlowKeycloak{t: t, execs: map[string][]keycloak.AuthenticationExecutionInfo{}}
}

func (f *fakeFlowKeycloak) id() string {
	f.nextID++
	return fmt.Sprintf("exec-%d", f.nextID)
}

func (f *fakeFlowKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/admin/realms/test/authentication/executions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/admin/realms/test/authentication/executions/")
		f.deleted = append(f.deleted, id)
		for alias, list := range f.execs {
			for i, e := range list {
				if e.ID != nil && *e.ID == id {
					f.execs[alias] = append(list[:i], list[i+1:]...)
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/admin/realms/test/authentication/flows/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/admin/realms/test/authentication/flows/")
		parts := strings.Split(rest, "/")
		alias := parts[0]
		tail := strings.Join(parts[1:], "/")
		switch {
		case r.Method == http.MethodGet && tail == "executions":
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(f.execs[alias])
			_, _ = w.Write(body)
		case r.Method == http.MethodPut && tail == "executions":
			var upd keycloak.AuthenticationExecutionInfo
			require.NoError(f.t, json.NewDecoder(r.Body).Decode(&upd))
			for i := range f.execs[alias] {
				if *f.execs[alias][i].ID == *upd.ID {
					if upd.Requirement != nil {
						f.execs[alias][i].Requirement = upd.Requirement
					}
					if upd.Priority != nil {
						f.execs[alias][i].Priority = upd.Priority
					}
				}
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && tail == "executions/flow":
			var def map[string]interface{}
			require.NoError(f.t, json.NewDecoder(r.Body).Decode(&def))
			f.subFlowDef = append(f.subFlowDef, def)
			subAlias, _ := def["alias"].(string)
			provider, _ := def["provider"].(string)
			f.execs[alias] = append(f.execs[alias], f.newExec(provider, subAlias, true))
			f.execs[subAlias] = nil
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && tail == "executions/execution":
			var body map[string]string
			require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
			f.execs[alias] = append(f.execs[alias], f.newExec(body["provider"], "", false))
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
	return mux
}

func (f *fakeFlowKeycloak) newExec(provider, displayName string, isFlow bool) keycloak.AuthenticationExecutionInfo {
	id := f.id()
	req := "DISABLED"
	level := 0
	e := keycloak.AuthenticationExecutionInfo{ID: &id, Requirement: &req, Level: &level, AuthenticationFlow: &isFlow}
	if provider != "" {
		e.ProviderID = &provider
	}
	if displayName != "" {
		e.DisplayName = &displayName
	}
	return e
}

func (f *fakeFlowKeycloak) client(baseURL string) *keycloak.Client {
	return keycloak.NewClient(keycloak.Config{BaseURL: baseURL, Realm: "master", ClientID: "admin-cli"}, testr.New(f.t))
}

func TestReconcileChildrenRecreatesFormFlowWithWrongAuthenticator(t *testing.T) {
	fake := newFakeFlowKeycloak(t)
	// Live state as produced before the fix: the form-flow execution carries
	// "form-flow" as its authenticator instead of a FormAuthenticator.
	broken := fake.newExec("form-flow", "reg-form", true)
	fake.execs["parent"] = []keycloak.AuthenticationExecutionInfo{broken}
	fake.execs["reg-form"] = []keycloak.AuthenticationExecutionInfo{fake.newExec("registration-user-creation", "", false)}

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	desired := []flowExecution{{
		SubFlow:     &flowDefinition{Alias: "reg-form", ProviderID: "form-flow"},
		Requirement: "REQUIRED",
		Executions:  []flowExecution{{Authenticator: "registration-user-creation", Requirement: "REQUIRED"}},
	}}

	r := &KeycloakAuthenticationFlowReconciler{}
	live, err := r.readLiveTree(context.Background(), kc, "test", "parent")
	require.NoError(t, err)
	require.Len(t, live, 1)
	require.Equal(t, "form-flow", live[0].Authenticator, "readLiveTree must expose the sub-flow execution's authenticator")

	stats := &updateStats{}
	require.NoError(t, r.reconcileChildren(context.Background(), kc, "test", "parent", desired, live, stats))

	require.Equal(t, []string{*broken.ID}, fake.deleted)
	require.Len(t, fake.subFlowDef, 1)
	require.Equal(t, "form-flow", fake.subFlowDef[0]["type"])
	require.Equal(t, "registration-page-form", fake.subFlowDef[0]["provider"])
	require.Equal(t, 1, stats.removed)
	require.Equal(t, 1, stats.added)

	parent := fake.execs["parent"]
	require.Len(t, parent, 1)
	require.Equal(t, "registration-page-form", *parent[0].ProviderID)
	require.Equal(t, "REQUIRED", *parent[0].Requirement)
	require.Len(t, fake.execs["reg-form"], 1)
	require.Equal(t, "registration-user-creation", *fake.execs["reg-form"][0].ProviderID)

	// Second pass must be a no-op.
	live, err = r.readLiveTree(context.Background(), kc, "test", "parent")
	require.NoError(t, err)
	stats = &updateStats{}
	require.NoError(t, r.reconcileChildren(context.Background(), kc, "test", "parent", desired, live, stats))
	require.False(t, stats.touched())
	require.Len(t, fake.deleted, 1)
}

func TestReconcileChildrenLeavesBasicFlowWithStrayAuthenticatorAlone(t *testing.T) {
	fake := newFakeFlowKeycloak(t)
	// basic-flow sub-flows created before the fix carry "basic-flow" as
	// authenticator; Keycloak ignores it, so no recreate must happen.
	sub := fake.newExec("basic-flow", "forms", true)
	req := "ALTERNATIVE"
	sub.Requirement = &req
	fake.execs["parent"] = []keycloak.AuthenticationExecutionInfo{sub}
	fake.execs["forms"] = nil

	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	kc := fake.client(srv.URL)

	desired := []flowExecution{{
		SubFlow:     &flowDefinition{Alias: "forms", ProviderID: "basic-flow"},
		Requirement: "ALTERNATIVE",
	}}

	r := &KeycloakAuthenticationFlowReconciler{}
	live, err := r.readLiveTree(context.Background(), kc, "test", "parent")
	require.NoError(t, err)
	stats := &updateStats{}
	require.NoError(t, r.reconcileChildren(context.Background(), kc, "test", "parent", desired, live, stats))
	require.False(t, stats.touched())
	require.Empty(t, fake.deleted)
}
