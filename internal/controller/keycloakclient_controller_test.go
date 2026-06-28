package controller

import (
	"encoding/json"
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

func TestDefinitionsMatch_ProtocolMappersSubset(t *testing.T) {
	// CR defines protocolMappers without id/consentRequired, KC adds those fields
	desired := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [{"name": "test-mapper", "protocol": "openid-connect", "protocolMapper": "oidc-usermodel-attribute-mapper", "config": {"claim.name": "test"}}]
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [{"id": "abc-123", "name": "test-mapper", "protocol": "openid-connect", "protocolMapper": "oidc-usermodel-attribute-mapper", "consentRequired": false, "config": {"claim.name": "test"}}]
	}`)

	if !definitionsMatch(desired, current) {
		t.Error("expected match: CR protocolMapper is a subset of KC protocolMapper")
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

func TestDefinitionsMatch_ProtocolMappersExtraInCurrent(t *testing.T) {
	// Keycloak has an extra protocolMapper that the CR no longer declares.
	// definitionsMatch must report drift so the PUT removes it.
	desired := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [{"name": "keep", "protocol": "openid-connect"}]
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [
			{"name": "keep", "protocol": "openid-connect"},
			{"name": "orphan", "protocol": "openid-connect"}
		]
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: orphan protocolMapper in current must surface as drift")
	}
}

func TestDefinitionsMatch_ProtocolMappersExtraInDesired(t *testing.T) {
	// CR adds a protocolMapper that Keycloak doesn't have yet — must surface as drift.
	desired := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [
			{"name": "existing", "protocol": "openid-connect"},
			{"name": "new-one", "protocol": "openid-connect"}
		]
	}`)
	current := json.RawMessage(`{
		"clientId": "app",
		"protocolMappers": [{"name": "existing", "protocol": "openid-connect"}]
	}`)

	if definitionsMatch(desired, current) {
		t.Error("expected no match: CR adds a protocolMapper that current lacks")
	}
}

func TestDefinitionsMatchStrict_DetectsRemovedField(t *testing.T) {
	// User had emailVerified: true in the CRD, then removed it.
	// Strict must detect that "emailVerified" is in current but not desired.
	desired := json.RawMessage(`{
		"username": "foo"
	}`)
	current := json.RawMessage(`{
		"username": "foo",
		"emailVerified": true
	}`)

	if definitionsMatchStrict(desired, current, "id", "createdTimestamp") {
		t.Error("expected no match: emailVerified is in current but not desired")
	}
}

func TestDefinitionsMatchStrict_IgnoresSystemFields(t *testing.T) {
	// id and createdTimestamp are known Keycloak-only fields — must be ignored.
	desired := json.RawMessage(`{
		"username": "foo",
		"enabled": true
	}`)
	current := json.RawMessage(`{
		"id": "abc-123",
		"createdTimestamp": "1718000000",
		"username": "foo",
		"enabled": true
	}`)

	if !definitionsMatchStrict(desired, current, "id", "createdTimestamp") {
		t.Error("expected match: system fields should be ignored")
	}
}

func TestDefinitionsMatchStrict_DetectsValueDiff(t *testing.T) {
	desired := json.RawMessage(`{
		"username": "foo",
		"enabled": true
	}`)
	current := json.RawMessage(`{
		"username": "foo",
		"enabled": false
	}`)

	if definitionsMatchStrict(desired, current, "id") {
		t.Error("expected no match: enabled differs")
	}
}

func TestDefinitionsMatchStrict_Match(t *testing.T) {
	desired := json.RawMessage(`{
		"username": "foo",
		"email": "foo@example.com",
		"enabled": true
	}`)
	current := json.RawMessage(`{
		"id": "xyz",
		"username": "foo",
		"email": "foo@example.com",
		"enabled": true,
		"createdTimestamp": 1718000000
	}`)

	if !definitionsMatchStrict(desired, current, "id", "createdTimestamp") {
		t.Error("expected match: all desired fields match and extra fields are system")
	}
}

func TestDefinitionsMatchStrict_DetectsNewFieldInCurrent(t *testing.T) {
	// A new field appears in Keycloak (e.g. from a manual edit) that is not
	// in desired and not in the ignore list — must detect as drift.
	desired := json.RawMessage(`{
		"username": "foo"
	}`)
	current := json.RawMessage(`{
		"username": "foo",
		"emailVerified": true
	}`)

	if definitionsMatchStrict(desired, current) {
		t.Error("expected no match: emailVerified is not in desired or ignore list")
	}
}

func TestDefinitionsMatchStrict_IgnoresRoleAndGroupFields(t *testing.T) {
	// realmRoles, clientRoles, and groups are reconciled via dedicated endpoints
	// and must be ignored in strict comparison to avoid spurious updates.
	desired := json.RawMessage(`{
		"username": "foo",
		"enabled": true
	}`)
	current := json.RawMessage(`{
		"id": "abc",
		"username": "foo",
		"enabled": true,
		"realmRoles": ["offline_access"],
		"clientRoles": {"my-app": ["admin"]},
		"groups": ["admins"]
	}`)

	if !definitionsMatchStrict(desired, current, "id", "realmRoles", "clientRoles", "groups") {
		t.Error("expected match: role and group fields should be ignored")
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
