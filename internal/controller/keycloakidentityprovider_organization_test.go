package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// fakeOrgLinkKeycloak stands in for the organization identity-provider link
// endpoints (GET/POST /organizations/{org}/identity-providers and
// DELETE .../identity-providers/{alias}). Links are kept per organization so
// the tests can assert both the resulting state and the number of writes.
type fakeOrgLinkKeycloak struct {
	t *testing.T
	// links maps an organization ID to the aliases linked to it. An
	// organization missing from the map answers 404, like a deleted one.
	links   map[string][]string
	posts   map[string]int
	deletes map[string]int
	gets    map[string]int
}

func newFakeOrgLinkKeycloak(t *testing.T) *fakeOrgLinkKeycloak {
	return &fakeOrgLinkKeycloak{
		t:       t,
		links:   map[string][]string{},
		posts:   map[string]int{},
		deletes: map[string]int{},
		gets:    map[string]int{},
	}
}

func (f *fakeOrgLinkKeycloak) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})

	const prefix = "/admin/realms/test/organizations/"
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, prefix)
		orgID, tail, _ := strings.Cut(rest, "/identity-providers")
		aliases, known := f.links[orgID]
		if !known {
			http.Error(w, `{"error":"Organization not found"}`, http.StatusNotFound)
			return
		}
		switch {
		case tail == "" && r.Method == http.MethodGet:
			f.gets[orgID]++
			reps := make([]keycloak.IdentityProviderRepresentation, 0, len(aliases))
			for _, a := range aliases {
				reps = append(reps, keycloak.IdentityProviderRepresentation{Alias: strPtr(a)})
			}
			writeUserJSON(w, reps)
		case tail == "" && r.Method == http.MethodPost:
			f.posts[orgID]++
			body, _ := io.ReadAll(r.Body)
			var alias string
			if err := json.Unmarshal(body, &alias); err != nil {
				http.Error(w, `{"error":"body must be a JSON string"}`, http.StatusBadRequest)
				return
			}
			for _, a := range aliases {
				if a == alias {
					http.Error(w, `{"error":"Identity provider already associated to the organization"}`, http.StatusConflict)
					return
				}
			}
			f.links[orgID] = append(aliases, alias)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(tail, "/") && r.Method == http.MethodDelete:
			f.deletes[orgID]++
			alias := strings.TrimPrefix(tail, "/")
			kept := aliases[:0:0]
			found := false
			for _, a := range aliases {
				if a == alias {
					found = true
					continue
				}
				kept = append(kept, a)
			}
			if !found {
				http.Error(w, `{"error":"Identity provider not found with the given alias"}`, http.StatusNotFound)
				return
			}
			f.links[orgID] = kept
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return mux
}

func (f *fakeOrgLinkKeycloak) client() *keycloak.Client {
	srv := httptest.NewServer(f.handler())
	f.t.Cleanup(srv.Close)
	return keycloak.NewClient(keycloak.Config{
		BaseURL:  srv.URL,
		Realm:    "master",
		ClientID: "admin-cli",
	}, testr.New(f.t))
}

func TestReconcileOrganizationLink(t *testing.T) {
	t.Parallel()
	const alias = "acme-sso"

	t.Run("links a new identity provider", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		fake.links["org-1"] = nil
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "", "org-1"))
		require.Equal(t, []string{alias}, fake.links["org-1"])
		require.Equal(t, 1, fake.posts["org-1"])
	})

	t.Run("re-reconcile is read-only", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		fake.links["org-1"] = nil
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "", "org-1"))
		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-1", "org-1"))
		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-1", "org-1"))
		require.Equal(t, 1, fake.posts["org-1"], "steady state must not POST again")
		require.Equal(t, 0, fake.deletes["org-1"])
		require.Equal(t, []string{alias}, fake.links["org-1"])
	})

	t.Run("restores a link dropped behind the operator's back", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		// status.organizationID says org-1, but Keycloak no longer lists the
		// IdP there (e.g. a 26.0 PUT without organizationId cleared it).
		fake.links["org-1"] = nil
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-1", "org-1"))
		require.Equal(t, []string{alias}, fake.links["org-1"])
		require.Equal(t, 1, fake.posts["org-1"])
	})

	t.Run("unlinks when organizationRef is removed", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		fake.links["org-1"] = []string{alias, "other-idp"}
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-1", ""))
		require.Equal(t, []string{"other-idp"}, fake.links["org-1"], "only the reconciled alias is unlinked")
		require.Equal(t, 1, fake.deletes["org-1"])
		require.Equal(t, 0, fake.posts["org-1"])

		// Second pass with the status already cleared touches nothing.
		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "", ""))
		require.Equal(t, 1, fake.deletes["org-1"])
		require.Equal(t, 0, fake.gets["org-1"]-1, "no organization is consulted without a ref or previous link")
	})

	t.Run("moves the link when organizationRef changes", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		fake.links["org-1"] = []string{alias}
		fake.links["org-2"] = nil
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-1", "org-2"))
		require.Empty(t, fake.links["org-1"])
		require.Equal(t, []string{alias}, fake.links["org-2"])
		require.Equal(t, 1, fake.deletes["org-1"])
		require.Equal(t, 1, fake.posts["org-2"])
	})

	t.Run("tolerates a previous organization that no longer exists", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		fake.links["org-2"] = nil
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		require.NoError(t, r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "org-gone", "org-2"))
		require.Equal(t, []string{alias}, fake.links["org-2"])
		require.Equal(t, 0, fake.deletes["org-gone"])
	})

	t.Run("surfaces a failed link", func(t *testing.T) {
		t.Parallel()
		fake := newFakeOrgLinkKeycloak(t)
		kc := fake.client()
		r := &KeycloakIdentityProviderReconciler{}

		err := r.reconcileOrganizationLink(context.Background(), kc, "test", alias, "", "org-missing")
		require.Error(t, err)
		require.Contains(t, err.Error(), "org-missing")
	})
}

func TestSameRealmPlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		idp  keycloakv1beta1.KeycloakIdentityProviderSpec
		org  keycloakv1beta1.KeycloakOrganizationSpec
		want bool
	}{
		{
			name: "matching realmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			want: true,
		},
		{
			name: "mismatched realmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-b"}},
			want: false,
		},
		{
			name: "matching clusterRealmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			want: true,
		},
		{
			name: "realmRef vs clusterRealmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "shared"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			want: false,
		},
		{
			name: "both unset",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idp := &keycloakv1beta1.KeycloakIdentityProvider{Spec: tc.idp}
			org := &keycloakv1beta1.KeycloakOrganization{Spec: tc.org}
			require.Equal(t, tc.want, sameRealmPlacement(idp, org))
		})
	}
}

func TestIsOrganizationRealmMismatch(t *testing.T) {
	t.Parallel()
	require.True(t, isOrganizationRealmMismatch(organizationRealmMismatchError{}))
	require.False(t, isOrganizationRealmMismatch(nil))
}
