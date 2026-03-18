package controller

import (
	"encoding/json"
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
