package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// strPtr is a tiny helper used by the drift table-tests to keep the rows
// readable. The IdentityProviderMapperRepresentation uses pointer fields
// across the board (id/name/identityProviderAlias/identityProviderMapper).
func strPtr(s string) *string { return &s }

func TestIdentityProviderAlias(t *testing.T) {
	t.Run("alias from definition", func(t *testing.T) {
		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "fallback"},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				Definition: runtime.RawExtension{Raw: []byte(`{"alias":"oidc","providerId":"oidc"}`)},
			},
		}
		alias, err := identityProviderAlias(idp)
		require.NoError(t, err)
		assert.Equal(t, "oidc", alias)
	})

	t.Run("falls back to metadata.name when alias missing", func(t *testing.T) {
		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "github"},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				Definition: runtime.RawExtension{Raw: []byte(`{"providerId":"github"}`)},
			},
		}
		alias, err := identityProviderAlias(idp)
		require.NoError(t, err)
		assert.Equal(t, "github", alias)
	})

	t.Run("falls back to metadata.name when alias empty string", func(t *testing.T) {
		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "saml-prod"},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				Definition: runtime.RawExtension{Raw: []byte(`{"alias":"","providerId":"saml"}`)},
			},
		}
		alias, err := identityProviderAlias(idp)
		require.NoError(t, err)
		assert.Equal(t, "saml-prod", alias)
	})

	t.Run("falls back to metadata.name on empty definition", func(t *testing.T) {
		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "empty"},
		}
		alias, err := identityProviderAlias(idp)
		require.NoError(t, err)
		assert.Equal(t, "empty", alias)
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "broken"},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				Definition: runtime.RawExtension{Raw: []byte(`not-json`)},
			},
		}
		_, err := identityProviderAlias(idp)
		require.Error(t, err)
	})
}

func TestIdentityProviderMapperDrifted(t *testing.T) {
	// The "desired" payload the controller would PUT — already enriched with
	// name and identityProviderAlias the way Reconcile() does.
	desired := json.RawMessage(`{
		"name": "username-mapper",
		"identityProviderAlias": "oidc",
		"identityProviderMapper": "oidc-username-idp-mapper",
		"config": {
			"template": "${ALIAS}.${CLAIM.sub}",
			"target": "LOCAL",
			"syncMode": "INHERIT"
		}
	}`)

	tests := []struct {
		name        string
		current     *keycloak.IdentityProviderMapperRepresentation
		wantDrift   bool
		wantErr     bool
		description string
	}{
		{
			name:        "nil current — never been created on Keycloak side, force update",
			current:     nil,
			wantDrift:   true,
			description: "first-create path goes through the no-existing-mapper branch upstream of this helper, but defensively this still returns drift=true",
		},
		{
			name: "identical content — in sync, skip update",
			current: &keycloak.IdentityProviderMapperRepresentation{
				ID:                     strPtr("uuid-from-keycloak"), // ignored by definitionsMatch
				Name:                   strPtr("username-mapper"),
				IdentityProviderAlias:  strPtr("oidc"),
				IdentityProviderMapper: strPtr("oidc-username-idp-mapper"),
				Config: map[string]string{
					"template": "${ALIAS}.${CLAIM.sub}",
					"target":   "LOCAL",
					"syncMode": "INHERIT",
				},
			},
			wantDrift:   false,
			description: "the typical re-reconcile case after creation: id is now populated, but nothing else changed",
		},
		{
			name: "config value diverged — drift",
			current: &keycloak.IdentityProviderMapperRepresentation{
				Name:                   strPtr("username-mapper"),
				IdentityProviderAlias:  strPtr("oidc"),
				IdentityProviderMapper: strPtr("oidc-username-idp-mapper"),
				Config: map[string]string{
					"template": "different-template",
					"target":   "LOCAL",
					"syncMode": "INHERIT",
				},
			},
			wantDrift:   true,
			description: "user changed the template in the CR — PUT must fire",
		},
		{
			name: "mapper type changed — drift",
			current: &keycloak.IdentityProviderMapperRepresentation{
				Name:                   strPtr("username-mapper"),
				IdentityProviderAlias:  strPtr("oidc"),
				IdentityProviderMapper: strPtr("oidc-attribute-idp-mapper"),
				Config: map[string]string{
					"template": "${ALIAS}.${CLAIM.sub}",
					"target":   "LOCAL",
					"syncMode": "INHERIT",
				},
			},
			wantDrift:   true,
			description: "the mapper type was edited; Keycloak needs the new value",
		},
		{
			name: "current has extra config keys Keycloak added itself — still in sync",
			current: &keycloak.IdentityProviderMapperRepresentation{
				Name:                   strPtr("username-mapper"),
				IdentityProviderAlias:  strPtr("oidc"),
				IdentityProviderMapper: strPtr("oidc-username-idp-mapper"),
				Config: map[string]string{
					"template":               "${ALIAS}.${CLAIM.sub}",
					"target":                 "LOCAL",
					"syncMode":               "INHERIT",
					"keycloak.internal.flag": "auto-set-by-server",
				},
			},
			wantDrift:   false,
			description: "definitionsMatch only checks that every key in the desired is present and equal in current; extra current keys are tolerated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drifted, err := identityProviderMapperDrifted(desired, tc.current)
			if tc.wantErr {
				require.Error(t, err, tc.description)
			} else {
				require.NoError(t, err, tc.description)
			}
			assert.Equal(t, tc.wantDrift, drifted, tc.description)
		})
	}
}
