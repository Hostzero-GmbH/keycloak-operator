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

	resource, err := transformer.TransformComponent(raw, "")
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

// Keycloak versions before 26 inline identityProviders and their mappers on
// the realm GET; those (and every other nested collection) are exported as
// their own CRDs and are rejected by the KeycloakRealm controller, so the
// exported realm must not carry them.
func TestTransformRealm_StripsNestedCollections(t *testing.T) {
	tr := NewTransformer(TransformerOptions{InstanceRef: "kc", TargetNamespace: "ns"})
	raw := json.RawMessage(`{
		"id": "abc",
		"realm": "demo",
		"enabled": true,
		"browserFlow": "custom-browser",
		"identityProviders": [{"alias": "github", "providerId": "github"}],
		"identityProviderMappers": [{"name": "m", "identityProviderAlias": "github"}],
		"clients": [{"clientId": "app"}],
		"authenticationFlows": [{"alias": "custom-browser"}],
		"components": {"org.keycloak.userprofile.UserProfileProvider": []}
	}`)
	res, err := tr.TransformRealm(raw, "demo")
	if err != nil {
		t.Fatalf("TransformRealm: %v", err)
	}
	realm := res.Object.(*keycloakv1beta1.KeycloakRealm)
	var def map[string]interface{}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &def); err != nil {
		t.Fatalf("definition is not JSON: %v", err)
	}
	for _, key := range append([]string{"id"}, keycloakv1beta1.RealmNestedCollectionKeys()...) {
		if _, present := def[key]; present {
			t.Errorf("exported realm definition still contains %q", key)
		}
	}
	if def["enabled"] != true || def["browserFlow"] != "custom-browser" {
		t.Errorf("top-level attributes were not preserved: %v", def)
	}
}

// A sub-component (an LDAP mapper under its provider) is exported with
// parentComponentRef instead of realmRef, keeps its parent's CR name as a
// prefix so sibling mappers under other providers do not collide, and still
// has parentId stripped: the controller injects it from the reference (#167).
func TestTransformComponentSubComponentParentRef(t *testing.T) {
	t.Parallel()

	transformer := NewTransformer(TransformerOptions{TargetNamespace: "ns", RealmRef: "my-realm"})

	provider, err := transformer.TransformComponent(json.RawMessage(`{
		"id": "ldap-id",
		"parentId": "realm-id",
		"name": "corporate-ldap",
		"providerId": "ldap",
		"providerType": "org.keycloak.storage.UserStorageProvider",
		"config": {"enabled": ["true"]}
	}`), "")
	require.NoError(t, err)
	require.Equal(t, "userstorageprovider-corporate-ldap", provider.Name)

	mapper, err := transformer.TransformComponent(json.RawMessage(`{
		"id": "mapper-id",
		"parentId": "ldap-id",
		"name": "username",
		"providerId": "user-attribute-ldap-mapper",
		"providerType": "org.keycloak.storage.ldap.mappers.LDAPStorageMapper",
		"config": {"ldap.attribute": ["uid"]}
	}`), provider.Name)
	require.NoError(t, err)

	component := mapper.Object.(*keycloakv1beta1.KeycloakComponent)
	require.Equal(t, "userstorageprovider-corporate-ldap-ldapstoragemapper-username", mapper.Name)
	require.Nil(t, component.Spec.RealmRef, "sub-component must not carry realmRef")
	require.Nil(t, component.Spec.ClusterRealmRef)
	require.NotNil(t, component.Spec.ParentComponentRef)
	require.Equal(t, provider.Name, component.Spec.ParentComponentRef.Name)
	require.Equal(t, "username", *component.Spec.Name)

	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(component.Spec.Definition.Raw, &def))
	_, hasParent := def["parentId"]
	require.False(t, hasParent, "parentId is injected by the controller")
	require.Equal(t, "org.keycloak.storage.ldap.mappers.LDAPStorageMapper", def["providerType"])
}

// A KeycloakRole or KeycloakGroup names exactly one parent; the CRDs reject a
// realmRef next to clientRef or parentGroupRef at admission, so an export
// that set both could never be applied.
func TestTransformRole_ClientRoleHasOnlyClientRef(t *testing.T) {
	tr := NewTransformer(TransformerOptions{InstanceRef: "kc", TargetNamespace: "ns", RealmRef: "realm"})
	res, err := tr.TransformRole(json.RawMessage(`{"id": "r1", "name": "editor", "containerId": "c1"}`), "app", "c1")
	require.NoError(t, err)
	role := res.Object.(*keycloakv1beta1.KeycloakRole)
	require.NotNil(t, role.Spec.ClientRef)
	require.Equal(t, "app", role.Spec.ClientRef.Name)
	require.Nil(t, role.Spec.RealmRef, "client role must not also carry realmRef")

	res, err = tr.TransformRole(json.RawMessage(`{"id": "r2", "name": "admin"}`), "", "")
	require.NoError(t, err)
	role = res.Object.(*keycloakv1beta1.KeycloakRole)
	require.Nil(t, role.Spec.ClientRef)
	require.NotNil(t, role.Spec.RealmRef)
	require.Equal(t, "realm", role.Spec.RealmRef.Name)
}

func TestTransformGroup_SubgroupHasOnlyParentGroupRef(t *testing.T) {
	tr := NewTransformer(TransformerOptions{InstanceRef: "kc", TargetNamespace: "ns", RealmRef: "realm"})
	res, err := tr.TransformGroup(json.RawMessage(`{"id": "g2", "name": "child", "path": "/parent/child", "parentId": "g1"}`), "parent")
	require.NoError(t, err)
	group := res.Object.(*keycloakv1beta1.KeycloakGroup)
	require.NotNil(t, group.Spec.ParentGroupRef)
	require.Equal(t, "parent", group.Spec.ParentGroupRef.Name)
	require.Nil(t, group.Spec.RealmRef, "subgroup must not also carry realmRef")
	require.NotContains(t, string(group.Spec.Definition.Raw), "parentId", "parent UUID must not leak into the definition")

	res, err = tr.TransformGroup(json.RawMessage(`{"id": "g1", "name": "parent", "path": "/parent"}`), "")
	require.NoError(t, err)
	group = res.Object.(*keycloakv1beta1.KeycloakGroup)
	require.Nil(t, group.Spec.ParentGroupRef)
	require.NotNil(t, group.Spec.RealmRef)
}
