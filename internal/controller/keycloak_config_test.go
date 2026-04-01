package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestMergeIDIntoDefinition(t *testing.T) {
	tests := []struct {
		name       string
		definition json.RawMessage
		id         *string
		want       string // expected JSON (will be compared after re-parsing)
		wantSame   bool   // expect original to be returned unchanged
	}{
		{
			name:       "adds id to empty object",
			definition: json.RawMessage(`{}`),
			id:         ptrString("123"),
			want:       `{"id":"123"}`,
		},
		{
			name:       "adds id to object with fields",
			definition: json.RawMessage(`{"name":"test","enabled":true}`),
			id:         ptrString("abc-123"),
			want:       `{"enabled":true,"id":"abc-123","name":"test"}`,
		},
		{
			name:       "overwrites existing id",
			definition: json.RawMessage(`{"id":"old-id","name":"test"}`),
			id:         ptrString("new-id"),
			want:       `{"id":"new-id","name":"test"}`,
		},
		{
			name:       "nil id returns original",
			definition: json.RawMessage(`{"name":"test"}`),
			id:         nil,
			wantSame:   true,
		},
		{
			name:       "empty id returns original",
			definition: json.RawMessage(`{"name":"test"}`),
			id:         ptrString(""),
			wantSame:   true,
		},
		{
			name:       "invalid JSON returns original",
			definition: json.RawMessage(`{invalid json`),
			id:         ptrString("123"),
			wantSame:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeIDIntoDefinition(tt.definition, tt.id)

			if tt.wantSame {
				if string(got) != string(tt.definition) {
					t.Errorf("expected original to be returned, got %s", string(got))
				}
				return
			}

			// Compare by parsing both as maps (order-independent comparison)
			var gotMap, wantMap map[string]interface{}
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("failed to parse expected: %v", err)
			}

			// Compare maps
			if len(gotMap) != len(wantMap) {
				t.Errorf("map length mismatch: got %d, want %d", len(gotMap), len(wantMap))
			}
			for k, v := range wantMap {
				if gotMap[k] != v {
					t.Errorf("field %q: got %v, want %v", k, gotMap[k], v)
				}
			}
		})
	}
}

func TestMergeSmtpCredentials(t *testing.T) {
	tests := []struct {
		name       string
		definition json.RawMessage
		user       string
		password   string
		wantUser   string
		wantPass   string
		wantHost   string // verify existing smtpServer fields are preserved
		wantSame   bool
	}{
		{
			name:       "injects into existing smtpServer",
			definition: json.RawMessage(`{"realm":"test","smtpServer":{"host":"smtp.example.com","port":"587"}}`),
			user:       "myuser",
			password:   "mypass",
			wantUser:   "myuser",
			wantPass:   "mypass",
			wantHost:   "smtp.example.com",
		},
		{
			name:       "creates smtpServer when missing",
			definition: json.RawMessage(`{"realm":"test"}`),
			user:       "user",
			password:   "pass",
			wantUser:   "user",
			wantPass:   "pass",
		},
		{
			name:       "overwrites existing user and password",
			definition: json.RawMessage(`{"smtpServer":{"host":"smtp.example.com","user":"old","password":"old"}}`),
			user:       "new-user",
			password:   "new-pass",
			wantUser:   "new-user",
			wantPass:   "new-pass",
			wantHost:   "smtp.example.com",
		},
		{
			name:       "injects into empty object",
			definition: json.RawMessage(`{}`),
			user:       "u",
			password:   "p",
			wantUser:   "u",
			wantPass:   "p",
		},
		{
			name:       "invalid JSON returns original",
			definition: json.RawMessage(`{invalid`),
			user:       "u",
			password:   "p",
			wantSame:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeSmtpCredentials(tt.definition, tt.user, tt.password)

			if tt.wantSame {
				if string(got) != string(tt.definition) {
					t.Errorf("expected original to be returned, got %s", string(got))
				}
				return
			}

			var defMap map[string]interface{}
			if err := json.Unmarshal(got, &defMap); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}

			smtp, ok := defMap["smtpServer"].(map[string]interface{})
			if !ok {
				t.Fatalf("smtpServer not found or not a map in result: %s", string(got))
			}

			if smtp["user"] != tt.wantUser {
				t.Errorf("user: got %v, want %v", smtp["user"], tt.wantUser)
			}
			if smtp["password"] != tt.wantPass {
				t.Errorf("password: got %v, want %v", smtp["password"], tt.wantPass)
			}
			if tt.wantHost != "" {
				if smtp["host"] != tt.wantHost {
					t.Errorf("host: got %v, want %v (existing fields should be preserved)", smtp["host"], tt.wantHost)
				}
			}
		})
	}
}

func TestMergeIDPConfig(t *testing.T) {
	tests := []struct {
		name       string
		definition json.RawMessage
		secretData map[string]string
		wantSame   bool
		check      func(t *testing.T, result json.RawMessage)
	}{
		{
			name:       "merges into existing config",
			definition: json.RawMessage(`{"alias":"my-idp","config":{"authorizationUrl":"https://idp.example.com/auth"}}`),
			secretData: map[string]string{"clientId": "my-client", "clientSecret": "my-secret"},
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				cfg := m["config"].(map[string]interface{})
				if cfg["clientId"] != "my-client" {
					t.Errorf("clientId: got %v, want my-client", cfg["clientId"])
				}
				if cfg["clientSecret"] != "my-secret" {
					t.Errorf("clientSecret: got %v, want my-secret", cfg["clientSecret"])
				}
				if cfg["authorizationUrl"] != "https://idp.example.com/auth" {
					t.Errorf("authorizationUrl should be preserved, got %v", cfg["authorizationUrl"])
				}
				if m["alias"] != "my-idp" {
					t.Errorf("alias should be preserved, got %v", m["alias"])
				}
			},
		},
		{
			name:       "creates config when missing",
			definition: json.RawMessage(`{"alias":"my-idp","providerId":"oidc"}`),
			secretData: map[string]string{"clientId": "new-client"},
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				cfg := m["config"].(map[string]interface{})
				if cfg["clientId"] != "new-client" {
					t.Errorf("clientId: got %v, want new-client", cfg["clientId"])
				}
			},
		},
		{
			name:       "secret values override inline config",
			definition: json.RawMessage(`{"config":{"clientId":"inline-id","clientSecret":"inline-secret","scope":"openid"}}`),
			secretData: map[string]string{"clientId": "secret-id", "clientSecret": "secret-secret"},
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				cfg := m["config"].(map[string]interface{})
				if cfg["clientId"] != "secret-id" {
					t.Errorf("clientId: got %v, want secret-id", cfg["clientId"])
				}
				if cfg["clientSecret"] != "secret-secret" {
					t.Errorf("clientSecret: got %v, want secret-secret", cfg["clientSecret"])
				}
				if cfg["scope"] != "openid" {
					t.Errorf("scope should be preserved, got %v", cfg["scope"])
				}
			},
		},
		{
			name:       "empty secretData returns original",
			definition: json.RawMessage(`{"alias":"test"}`),
			secretData: map[string]string{},
			wantSame:   true,
		},
		{
			name:       "nil secretData returns original",
			definition: json.RawMessage(`{"alias":"test"}`),
			secretData: nil,
			wantSame:   true,
		},
		{
			name:       "invalid JSON returns original",
			definition: json.RawMessage(`{invalid`),
			secretData: map[string]string{"clientId": "test"},
			wantSame:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeIDPConfig(tt.definition, tt.secretData)

			if tt.wantSame {
				if string(got) != string(tt.definition) {
					t.Errorf("expected original to be returned, got %s", string(got))
				}
				return
			}

			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestSetFieldInDefinition(t *testing.T) {
	tests := []struct {
		name       string
		definition json.RawMessage
		field      string
		value      interface{}
		want       string
	}{
		{
			name:       "sets string field",
			definition: json.RawMessage(`{"name":"test"}`),
			field:      "realm",
			value:      "my-realm",
			want:       `{"name":"test","realm":"my-realm"}`,
		},
		{
			name:       "sets bool field",
			definition: json.RawMessage(`{"name":"test"}`),
			field:      "enabled",
			value:      true,
			want:       `{"enabled":true,"name":"test"}`,
		},
		{
			name:       "overwrites existing field",
			definition: json.RawMessage(`{"name":"old"}`),
			field:      "name",
			value:      "new",
			want:       `{"name":"new"}`,
		},
		{
			name:       "sets field on empty object",
			definition: json.RawMessage(`{}`),
			field:      "key",
			value:      "value",
			want:       `{"key":"value"}`,
		},
		{
			name:       "creates map for invalid JSON",
			definition: json.RawMessage(`invalid`),
			field:      "key",
			value:      "value",
			want:       `{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setFieldInDefinition(tt.definition, tt.field, tt.value)

			// Compare by parsing both as maps
			var gotMap, wantMap map[string]interface{}
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("failed to parse expected: %v", err)
			}

			if len(gotMap) != len(wantMap) {
				t.Errorf("map length mismatch: got %d, want %d", len(gotMap), len(wantMap))
			}
			for k, v := range wantMap {
				if gotMap[k] != v {
					t.Errorf("field %q: got %v, want %v", k, gotMap[k], v)
				}
			}
		})
	}
}

func TestResolveFlowBindingAliases(t *testing.T) {
	ctx := context.Background()
	realmName := "test-realm"

	fakeLookup := func(_ context.Context, _ string, alias string) (string, error) {
		switch alias {
		case "custom-browser-flow":
			return "uuid-browser-1234", nil
		case "custom-direct-grant":
			return "uuid-direct-5678", nil
		default:
			return "", fmt.Errorf("authentication flow not found: %s", alias)
		}
	}

	tests := []struct {
		name       string
		definition string
		wantErr    bool
		errContain string
		check      func(t *testing.T, result json.RawMessage)
	}{
		{
			name:       "no authenticationFlowBindingOverrides passes through",
			definition: `{"clientId":"my-client","enabled":true}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				if _, ok := m["authenticationFlowBindingOverrides"]; ok {
					t.Error("expected no authenticationFlowBindingOverrides")
				}
			},
		},
		{
			name:       "empty overrides passes through",
			definition: `{"clientId":"my-client","authenticationFlowBindingOverrides":{}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if len(overrides) != 0 {
					t.Errorf("expected empty overrides, got %v", overrides)
				}
			},
		},
		{
			name:       "UUID-based keys pass through unchanged",
			definition: `{"authenticationFlowBindingOverrides":{"browser":"existing-uuid","direct_grant":"another-uuid"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["browser"] != "existing-uuid" {
					t.Errorf("browser: got %v, want existing-uuid", overrides["browser"])
				}
				if overrides["direct_grant"] != "another-uuid" {
					t.Errorf("direct_grant: got %v, want another-uuid", overrides["direct_grant"])
				}
			},
		},
		{
			name:       "browserFlowAlias resolves to browser UUID",
			definition: `{"authenticationFlowBindingOverrides":{"browserFlowAlias":"custom-browser-flow"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["browser"] != "uuid-browser-1234" {
					t.Errorf("browser: got %v, want uuid-browser-1234", overrides["browser"])
				}
				if _, ok := overrides["browserFlowAlias"]; ok {
					t.Error("browserFlowAlias should have been removed")
				}
			},
		},
		{
			name:       "directGrantFlowAlias resolves to direct_grant UUID",
			definition: `{"authenticationFlowBindingOverrides":{"directGrantFlowAlias":"custom-direct-grant"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["direct_grant"] != "uuid-direct-5678" {
					t.Errorf("direct_grant: got %v, want uuid-direct-5678", overrides["direct_grant"])
				}
				if _, ok := overrides["directGrantFlowAlias"]; ok {
					t.Error("directGrantFlowAlias should have been removed")
				}
			},
		},
		{
			name:       "both aliases resolve together",
			definition: `{"authenticationFlowBindingOverrides":{"browserFlowAlias":"custom-browser-flow","directGrantFlowAlias":"custom-direct-grant"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["browser"] != "uuid-browser-1234" {
					t.Errorf("browser: got %v, want uuid-browser-1234", overrides["browser"])
				}
				if overrides["direct_grant"] != "uuid-direct-5678" {
					t.Errorf("direct_grant: got %v, want uuid-direct-5678", overrides["direct_grant"])
				}
			},
		},
		{
			name:       "alias takes precedence over UUID key",
			definition: `{"authenticationFlowBindingOverrides":{"browser":"old-uuid","browserFlowAlias":"custom-browser-flow"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["browser"] != "uuid-browser-1234" {
					t.Errorf("browser: got %v, want uuid-browser-1234 (alias should win)", overrides["browser"])
				}
			},
		},
		{
			name:       "unknown alias returns error",
			definition: `{"authenticationFlowBindingOverrides":{"browserFlowAlias":"nonexistent-flow"}}`,
			wantErr:    true,
			errContain: "nonexistent-flow",
		},
		{
			name:       "empty alias value returns error",
			definition: `{"authenticationFlowBindingOverrides":{"browserFlowAlias":""}}`,
			wantErr:    true,
			errContain: "non-empty string",
		},
		{
			name:       "invalid JSON passes through unchanged",
			definition: `{invalid`,
			check: func(t *testing.T, result json.RawMessage) {
				if string(result) != `{invalid` {
					t.Errorf("expected original to be returned, got %s", string(result))
				}
			},
		},
		{
			name:       "non-map overrides passes through unchanged",
			definition: `{"authenticationFlowBindingOverrides":"not-a-map"}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				if m["authenticationFlowBindingOverrides"] != "not-a-map" {
					t.Errorf("expected unchanged, got %v", m["authenticationFlowBindingOverrides"])
				}
			},
		},
		{
			name:       "other definition fields are preserved",
			definition: `{"clientId":"test","enabled":true,"authenticationFlowBindingOverrides":{"browserFlowAlias":"custom-browser-flow"}}`,
			check: func(t *testing.T, result json.RawMessage) {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatal(err)
				}
				if m["clientId"] != "test" {
					t.Errorf("clientId should be preserved, got %v", m["clientId"])
				}
				if m["enabled"] != true {
					t.Errorf("enabled should be preserved, got %v", m["enabled"])
				}
				overrides := m["authenticationFlowBindingOverrides"].(map[string]interface{})
				if overrides["browser"] != "uuid-browser-1234" {
					t.Errorf("browser: got %v, want uuid-browser-1234", overrides["browser"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveFlowBindingAliasesWithLookup(ctx, realmName, json.RawMessage(tt.definition), fakeLookup)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContain != "" {
					errStr := err.Error()
					found := false
					for i := 0; i <= len(errStr)-len(tt.errContain); i++ {
						if errStr[i:i+len(tt.errContain)] == tt.errContain {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("error %q should contain %q", errStr, tt.errContain)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
