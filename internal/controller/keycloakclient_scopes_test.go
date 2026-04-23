package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

func TestExtractStringSliceFromDefinition(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		field      string
		wantValues []string
		wantExists bool
	}{
		{
			name:       "field present with values",
			definition: `{"defaultClientScopes":["openid","email","profile"]}`,
			field:      "defaultClientScopes",
			wantValues: []string{"openid", "email", "profile"},
			wantExists: true,
		},
		{
			name:       "field present as empty array",
			definition: `{"defaultClientScopes":[]}`,
			field:      "defaultClientScopes",
			wantValues: []string{},
			wantExists: true,
		},
		{
			name:       "field absent",
			definition: `{"clientId":"my-app"}`,
			field:      "defaultClientScopes",
			wantValues: nil,
			wantExists: false,
		},
		{
			name:       "field is null",
			definition: `{"defaultClientScopes":null}`,
			field:      "defaultClientScopes",
			wantValues: nil,
			wantExists: false,
		},
		{
			name:       "invalid JSON",
			definition: `{invalid`,
			field:      "defaultClientScopes",
			wantValues: nil,
			wantExists: false,
		},
		{
			name:       "field is not an array",
			definition: `{"defaultClientScopes":"not-an-array"}`,
			field:      "defaultClientScopes",
			wantValues: nil,
			wantExists: false,
		},
		{
			name:       "optionalClientScopes with values",
			definition: `{"optionalClientScopes":["phone","address"]}`,
			field:      "optionalClientScopes",
			wantValues: []string{"phone", "address"},
			wantExists: true,
		},
		{
			name:       "both fields present extracts correct one",
			definition: `{"defaultClientScopes":["openid"],"optionalClientScopes":["phone"]}`,
			field:      "defaultClientScopes",
			wantValues: []string{"openid"},
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, exists := extractStringSliceFromDefinition([]byte(tt.definition), tt.field)
			if exists != tt.wantExists {
				t.Errorf("exists: got %v, want %v", exists, tt.wantExists)
			}
			if tt.wantValues == nil {
				if values != nil {
					t.Errorf("values: got %v, want nil", values)
				}
			} else {
				if len(values) != len(tt.wantValues) {
					t.Fatalf("values length: got %d, want %d", len(values), len(tt.wantValues))
				}
				for i, v := range values {
					if v != tt.wantValues[i] {
						t.Errorf("values[%d]: got %q, want %q", i, v, tt.wantValues[i])
					}
				}
			}
		})
	}
}

type scopeAction struct {
	action string // "add" or "remove"
	name   string
}

func TestReconcileScopeAssignments(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	r := &KeycloakClientReconciler{}

	realmScopes := map[string]string{
		"openid":  "id-openid",
		"email":   "id-email",
		"profile": "id-profile",
		"phone":   "id-phone",
		"address": "id-address",
		"roles":   "id-roles",
	}

	tests := []struct {
		name        string
		current     []keycloak.ClientScopeRepresentation
		desired     []string
		wantActions []scopeAction
		wantErr     bool
		errContains string
	}{
		{
			name: "no changes needed when current matches desired",
			current: []keycloak.ClientScopeRepresentation{
				{ID: ptr("id-openid"), Name: ptr("openid")},
				{ID: ptr("id-email"), Name: ptr("email")},
			},
			desired:     []string{"openid", "email"},
			wantActions: nil,
		},
		{
			name: "adds missing scopes",
			current: []keycloak.ClientScopeRepresentation{
				{ID: ptr("id-openid"), Name: ptr("openid")},
			},
			desired: []string{"openid", "email", "profile"},
			wantActions: []scopeAction{
				{action: "add", name: "email"},
				{action: "add", name: "profile"},
			},
		},
		{
			name: "removes extra scopes",
			current: []keycloak.ClientScopeRepresentation{
				{ID: ptr("id-openid"), Name: ptr("openid")},
				{ID: ptr("id-email"), Name: ptr("email")},
				{ID: ptr("id-phone"), Name: ptr("phone")},
			},
			desired: []string{"openid"},
			wantActions: []scopeAction{
				{action: "remove", name: "email"},
				{action: "remove", name: "phone"},
			},
		},
		{
			name: "removes all when desired is empty",
			current: []keycloak.ClientScopeRepresentation{
				{ID: ptr("id-openid"), Name: ptr("openid")},
				{ID: ptr("id-email"), Name: ptr("email")},
			},
			desired: []string{},
			wantActions: []scopeAction{
				{action: "remove", name: "openid"},
				{action: "remove", name: "email"},
			},
		},
		{
			name:    "adds all when current is empty",
			current: []keycloak.ClientScopeRepresentation{},
			desired: []string{"openid", "email"},
			wantActions: []scopeAction{
				{action: "add", name: "openid"},
				{action: "add", name: "email"},
			},
		},
		{
			name: "mixed add and remove",
			current: []keycloak.ClientScopeRepresentation{
				{ID: ptr("id-email"), Name: ptr("email")},
				{ID: ptr("id-phone"), Name: ptr("phone")},
			},
			desired: []string{"email", "profile", "roles"},
			wantActions: []scopeAction{
				{action: "remove", name: "phone"},
				{action: "add", name: "profile"},
				{action: "add", name: "roles"},
			},
		},
		{
			name:        "error when desired scope does not exist in realm",
			current:     []keycloak.ClientScopeRepresentation{},
			desired:     []string{"nonexistent-scope"},
			wantErr:     true,
			errContains: "does not exist in realm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actions []scopeAction

			getCurrent := func(_ context.Context, _, _ string) ([]keycloak.ClientScopeRepresentation, error) {
				return tt.current, nil
			}
			addScope := func(_ context.Context, _, _, scopeID string) error {
				for name, id := range realmScopes {
					if id == scopeID {
						actions = append(actions, scopeAction{action: "add", name: name})
						return nil
					}
				}
				return fmt.Errorf("unknown scope ID: %s", scopeID)
			}
			removeScope := func(_ context.Context, _, _, scopeID string) error {
				for name, id := range realmScopes {
					if id == scopeID {
						actions = append(actions, scopeAction{action: "remove", name: name})
						return nil
					}
				}
				return fmt.Errorf("unknown scope ID: %s", scopeID)
			}

			err := r.reconcileScopeAssignments(ctx, log, nil, "test-realm", "client-uuid", "default",
				tt.desired, realmScopes, getCurrent, addScope, removeScope)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantActions == nil {
				if len(actions) != 0 {
					t.Errorf("expected no actions, got %v", actions)
				}
				return
			}

			// Build sets for comparison (order of removes/adds may vary due to map iteration)
			wantSet := make(map[scopeAction]bool)
			for _, a := range tt.wantActions {
				wantSet[a] = true
			}
			gotSet := make(map[scopeAction]bool)
			for _, a := range actions {
				gotSet[a] = true
			}

			if len(gotSet) != len(wantSet) {
				t.Errorf("actions count mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(gotSet), len(wantSet), actions, tt.wantActions)
			}
			for a := range wantSet {
				if !gotSet[a] {
					t.Errorf("missing action: %v", a)
				}
			}
			for a := range gotSet {
				if !wantSet[a] {
					t.Errorf("unexpected action: %v", a)
				}
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
