package controller

import (
	"encoding/json"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestDefinitionsMatch_ScopesUnordered(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": ["basic", "scope-a", "scope-b", "scope-c"]
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": ["scope-c", "scope-a", "basic", "scope-b"],
		"access": {"configure": true}
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: same scopes in different order should be equal")
	}
}

func TestDefinitionsMatch_ScalarFieldDiff(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "user",
		"publicClient": true
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"publicClient": false
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: publicClient differs")
	}
}

func TestDefinitionsMatch_ExtraFieldsIgnored(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "user"
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"access": {"configure": true},
		"attributes": {"realm_client": "false"}
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: extra fields in Keycloak should be ignored")
	}
}

func TestDefinitionsMatch_RedirectUrisUnordered(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "app",
		"redirectUris": ["https://a.com/*", "https://b.com/*"]
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"redirectUris": ["https://b.com/*", "https://a.com/*"]
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: same redirectUris in different order should be equal")
	}
}

func TestDefinitionsMatch_EmptyScopes(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": []
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": []
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: both empty scopes")
	}
}

func TestDefinitionsMatch_AttributesSubset(t *testing.T) {
	// CR defines a subset of attributes, Keycloak adds SAML defaults — should match
	desired := json.RawMessage(`{
		"clientId": "app",
		"attributes": {"oauth2.device.authorization.grant.enabled": "false", "post.logout.redirect.uris": "+"}
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"attributes": {"saml.assertion.signature": "false", "saml.force.post.binding": "false", "oauth2.device.authorization.grant.enabled": "false", "post.logout.redirect.uris": "+"}
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: CR attributes are a subset of Keycloak attributes")
	}
}

func TestDefinitionsMatch_AttributeValueDiff(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "app",
		"attributes": {"oauth2.device.authorization.grant.enabled": "true"}
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"attributes": {"oauth2.device.authorization.grant.enabled": "false"}
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: attribute value differs")
	}
}

func TestDefinitionsMatch_ObjectArraySubset(t *testing.T) {
	// CR defines an object array without the fields Keycloak adds on read (id, ...)
	desired := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"name": "res-a", "type": "urn:app:resources:default"}]}
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"id": "abc-123", "name": "res-a", "type": "urn:app:resources:default", "ownerManagedAccess": false}]}
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: CR object is a subset of the Keycloak object")
	}
}

func TestDefinitionsMatch_SkipsDefaultClientScopes(t *testing.T) {
	// defaultClientScopes are synced via dedicated API, so definitionsMatch should ignore them
	desired := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": ["scope-a", "scope-b", "scope-c"],
		"publicClient": false
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"defaultClientScopes": ["scope-x"],
		"publicClient": false
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: defaultClientScopes should be skipped in comparison")
	}
}

func TestDefinitionsMatch_SkipsOptionalClientScopes(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "user",
		"optionalClientScopes": ["opt-a"],
		"publicClient": true
	}`)
	current := json.RawMessage(`{
		"clientId": "user",
		"optionalClientScopes": [],
		"publicClient": true
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: optionalClientScopes should be skipped in comparison")
	}
}

func TestDefinitionsMatch_ObjectArrayExtraInCurrent(t *testing.T) {
	// Keycloak has an extra object that the CR no longer declares.
	// definitionsMatch must report drift so the PUT removes it.
	desired := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"name": "keep"}]}
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"name": "keep"}, {"name": "orphan"}]}
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: orphan object in current must surface as drift")
	}
}

func TestDefinitionsMatch_ObjectArrayExtraInDesired(t *testing.T) {
	// CR adds an object that Keycloak doesn't have yet — must surface as drift.
	desired := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"name": "existing"}, {"name": "new-one"}]}
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"authorizationSettings": {"resources": [{"name": "existing"}]}
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: CR adds an object that current lacks")
	}
}

func TestDefinitionDriftKeys_NamesDifferingKeys(t *testing.T) {
	// Every differing top-level key is named, sorted; matching keys and keys
	// that only Keycloak knows about are left out.
	desired := json.RawMessage(`{
		"clientId": "app",
		"webOrigins": ["https://a.example.com"],
		"attributes": {"post.logout.redirect.uris": "https://a.example.com/*"},
		"description": "new"
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"webOrigins": ["https://b.example.com"],
		"attributes": {"post.logout.redirect.uris": "https://b.example.com/*"},
		"description": "new",
		"access": {"configure": true}
	}`)

	keys, err := definitionDriftKeys(desired, current)
	if err != nil {
		t.Fatalf("definitionDriftKeys: %v", err)
	}
	if want := []string{"attributes", "webOrigins"}; !slices.Equal(keys, want) {
		t.Errorf("definitionDriftKeys = %v, want %v", keys, want)
	}
}

func TestDefinitionDriftKeys_MissingKeyIsDrift(t *testing.T) {
	desired := json.RawMessage(`{"clientId": "app", "description": "set by the CR"}`)
	current := json.RawMessage(`{"clientId": "app"}`)

	keys, err := definitionDriftKeys(desired, current)
	if err != nil {
		t.Fatalf("definitionDriftKeys: %v", err)
	}
	if want := []string{"description"}; !slices.Equal(keys, want) {
		t.Errorf("definitionDriftKeys = %v, want %v", keys, want)
	}
}

func TestDefinitionDriftKeys_InSyncNamesNothing(t *testing.T) {
	desired := json.RawMessage(`{
		"clientId": "app",
		"redirectUris": ["https://a/*", "https://b/*"],
		"defaultClientScopes": ["profile"]
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"redirectUris": ["https://b/*", "https://a/*"],
		"defaultClientScopes": ["email"]
	}`)

	keys, err := definitionDriftKeys(desired, current)
	if err != nil {
		t.Fatalf("definitionDriftKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected no drift (unordered arrays, scope assignments skipped), got %v", keys)
	}
}

func TestDefinitionDriftKeys_InvalidJSON(t *testing.T) {
	valid := json.RawMessage(`{"clientId": "app"}`)

	if _, err := definitionDriftKeys(json.RawMessage(`not json`), valid); err == nil {
		t.Error("expected an error for an invalid desired definition")
	}
	if _, err := definitionDriftKeys(valid, nil); err == nil {
		t.Error("expected an error for a missing current state")
	}
	if definitionsMatch(valid, nil) {
		t.Error("expected no match when the current state is missing")
	}
}

func TestIsPublicClient(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "public", raw: `{"clientId":"app","publicClient":true}`, want: true},
		{name: "explicit confidential", raw: `{"clientId":"app","publicClient":false}`, want: false},
		{name: "field absent", raw: `{"clientId":"app"}`, want: false},
		{name: "definition empty", raw: ``, want: false},
		{name: "definition garbage", raw: `not json`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kc := &keycloakv1beta1.KeycloakClient{}
			if tc.raw != "" {
				kc.Spec.Definition = &runtime.RawExtension{Raw: []byte(tc.raw)}
			}
			if got := isPublicClient(kc); got != tc.want {
				t.Errorf("isPublicClient(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsPublicClient_NilSpec(t *testing.T) {
	kc := &keycloakv1beta1.KeycloakClient{}
	if isPublicClient(kc) {
		t.Error("expected false for nil Definition")
	}
}

func TestIsPublicClient_EmptyRaw(t *testing.T) {
	// Definition is non-nil but Raw is empty — exercises the
	// len(kcClient.Spec.Definition.Raw) == 0 branch that the table-driven
	// "definition empty" case skips (since it leaves Definition nil).
	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "nil raw", raw: nil},
		{name: "empty raw", raw: []byte{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kc := &keycloakv1beta1.KeycloakClient{}
			kc.Spec.Definition = &runtime.RawExtension{Raw: tc.raw}
			if isPublicClient(kc) {
				t.Errorf("expected false for non-nil Definition with %s", tc.name)
			}
		})
	}
}
