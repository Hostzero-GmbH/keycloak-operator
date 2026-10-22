package controller

import (
	"strings"
	"testing"
)

func TestRejectDefinitionKey(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		wantErr    bool
	}{
		{
			name:       "absent key passes",
			definition: `{"protocol": "openid-connect"}`,
		},
		{
			name:       "empty definition passes",
			definition: ``,
		},
		{
			name:       "malformed definition defers to caller parse",
			definition: `{not json`,
		},
		{
			name:       "present key is rejected",
			definition: `{"protocol": "openid-connect", "protocolMappers": [{"name": "m2m"}]}`,
			wantErr:    true,
		},
		{
			name:       "empty array still counts as present",
			definition: `{"protocolMappers": []}`,
			wantErr:    true,
		},
		{
			name:       "null still counts as present",
			definition: `{"protocolMappers": null}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectDefinitionKey([]byte(tt.definition), "protocolMappers", "KeycloakProtocolMapper")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), "KeycloakProtocolMapper") {
					t.Errorf("error should name the owning CRD, got %q", err)
				}
				return
			}
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestRejectRealmNestedCollections(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		wantKind   string // empty: expect no error
	}{
		{
			name:       "top-level attributes and flow bindings pass",
			definition: `{"enabled": true, "browserFlow": "custom-browser", "attributes": {"x": "y"}}`,
		},
		{
			name:       "empty definition passes",
			definition: ``,
		},
		{
			name:       "authenticationFlows is rejected",
			definition: `{"authenticationFlows": [{"alias": "custom-browser"}]}`,
			wantKind:   "KeycloakAuthenticationFlow",
		},
		{
			name:       "components is rejected",
			definition: `{"components": {"org.keycloak.userprofile.UserProfileProvider": []}}`,
			wantKind:   "KeycloakComponent",
		},
		{
			name:       "roles object form is rejected",
			definition: `{"roles": {"realm": [], "client": {}}}`,
			wantKind:   "KeycloakRole",
		},
		{
			name:       "clients is rejected",
			definition: `{"clients": []}`,
			wantKind:   "KeycloakClient",
		},
		{
			name:       "first key in sorted order is reported",
			definition: `{"users": [], "components": {}}`,
			wantKind:   "KeycloakComponent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectRealmNestedCollections([]byte(tt.definition))
			if tt.wantKind == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error naming %s, got nil", tt.wantKind)
			}
			if !strings.Contains(err.Error(), tt.wantKind) {
				t.Errorf("error should name %s, got %q", tt.wantKind, err)
			}
		})
	}
}

// Every key in the table must point at a CRD kind this operator ships.
func TestRealmNestedCollectionsNameKnownKinds(t *testing.T) {
	known := map[string]bool{
		"KeycloakAuthenticationFlow": true, "KeycloakClient": true, "KeycloakClientScope": true,
		"KeycloakComponent": true, "KeycloakGroup": true, "KeycloakIdentityProvider": true,
		"KeycloakIdentityProviderMapper": true, "KeycloakOrganization": true,
		"KeycloakRequiredAction": true, "KeycloakRole": true, "KeycloakUser": true,
	}
	for key, kind := range realmNestedCollections {
		if !known[kind] {
			t.Errorf("%s maps to unknown kind %q", key, kind)
		}
	}
}
