package controller

import (
	"encoding/json"
	"testing"
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
