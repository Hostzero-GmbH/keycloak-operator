package export

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestTransformIdentityProviderSetsAliasAndStripsOrganizationID(t *testing.T) {
	t.Parallel()

	transformer := NewTransformer(TransformerOptions{
		TargetNamespace: "ns",
		RealmRef:        "my-realm",
	})
	transformer.SetOrganizationNames(map[string]string{
		"org-uuid-1": "acme-corp",
	})

	raw := json.RawMessage(`{
		"alias": "acme-sso",
		"internalId": "idp-internal",
		"organizationId": "org-uuid-1",
		"providerId": "oidc",
		"config": {"clientSecret": "super-secret"}
	}`)

	resource, err := transformer.TransformIdentityProvider(raw)
	require.NoError(t, err)

	idp, ok := resource.Object.(*keycloakv1beta1.KeycloakIdentityProvider)
	require.True(t, ok)
	require.Equal(t, "acme-sso", resource.Name)
	require.NotNil(t, idp.Spec.Alias)
	require.Equal(t, "acme-sso", *idp.Spec.Alias)
	require.NotNil(t, idp.Spec.OrganizationRef)
	require.Equal(t, "acme-corp", idp.Spec.OrganizationRef.Name)

	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(idp.Spec.Definition.Raw, &def))
	_, hasOrgID := def["organizationId"]
	require.False(t, hasOrgID, "organizationId must be stripped from definition")
	_, hasInternal := def["internalId"]
	require.False(t, hasInternal)
	cfg, _ := def["config"].(map[string]interface{})
	_, hasSecret := cfg["clientSecret"]
	require.False(t, hasSecret)
}

func TestTransformIdentityProviderDropsUnresolvedOrganizationID(t *testing.T) {
	t.Parallel()

	transformer := NewTransformer(TransformerOptions{
		TargetNamespace: "ns",
		RealmRef:        "my-realm",
	})

	raw := json.RawMessage(`{
		"alias": "orphan-sso",
		"organizationId": "missing-org",
		"providerId": "oidc"
	}`)

	resource, err := transformer.TransformIdentityProvider(raw)
	require.NoError(t, err)

	idp := resource.Object.(*keycloakv1beta1.KeycloakIdentityProvider)
	require.Nil(t, idp.Spec.OrganizationRef)
	require.NotNil(t, idp.Spec.Alias)
	require.Equal(t, "orphan-sso", *idp.Spec.Alias)

	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(idp.Spec.Definition.Raw, &def))
	_, hasOrgID := def["organizationId"]
	require.False(t, hasOrgID)
}

func TestTransformComponentStripsBindCredential(t *testing.T) {
	t.Parallel()

	transformer := NewTransformer(TransformerOptions{
		TargetNamespace: "ns",
		RealmRef:        "my-realm",
	})

	raw := json.RawMessage(`{
		"id": "comp-id",
		"parentId": "realm-id",
		"name": "corporate-ldap",
		"providerId": "ldap",
		"providerType": "org.keycloak.storage.UserStorageProvider",
		"config": {
			"enabled": ["true"],
			"bindCredential": ["super-secret"]
		}
	}`)

	resource, err := transformer.TransformComponent(raw)
	require.NoError(t, err)

	component := resource.Object.(*keycloakv1beta1.KeycloakComponent)
	require.NotNil(t, component.Spec.Name)
	require.Equal(t, "corporate-ldap", *component.Spec.Name)
	require.Nil(t, component.Spec.ConfigSecretRef)

	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(component.Spec.Definition.Raw, &def))
	_, hasID := def["id"]
	require.False(t, hasID)
	_, hasParent := def["parentId"]
	require.False(t, hasParent)
	cfg, _ := def["config"].(map[string]interface{})
	_, hasBind := cfg["bindCredential"]
	require.False(t, hasBind, "bindCredential must be stripped from exported definition")
	enabled, ok := cfg["enabled"].([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"true"}, enabled)
}

func TestTransformOrganizationSetsName(t *testing.T) {
	t.Parallel()

	transformer := NewTransformer(TransformerOptions{
		TargetNamespace: "ns",
		RealmRef:        "my-realm",
	})

	raw := json.RawMessage(`{"id": "org-uuid", "name": "ACME Corp", "enabled": true}`)
	resource, err := transformer.TransformOrganization(raw)
	require.NoError(t, err)

	org := resource.Object.(*keycloakv1beta1.KeycloakOrganization)
	require.Equal(t, "acme-corp", resource.Name)
	require.NotNil(t, org.Spec.Name)
	require.Equal(t, "ACME Corp", *org.Spec.Name)

	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(org.Spec.Definition.Raw, &def))
	_, hasID := def["id"]
	require.False(t, hasID)
}
