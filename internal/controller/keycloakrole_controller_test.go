package controller

import (
	"encoding/json"
	"testing"
)

// The role controller builds its PUT body from spec.definition with name and id
// merged in and composites stripped, then gates the PUT on definitionsMatch
// against the raw GET. These cases pin the drift semantics for role-shaped JSON.
func TestRoleDefinitionsMatch(t *testing.T) {
	current := json.RawMessage(`{
		"id": "526f",
		"name": "hostzero-staff",
		"description": "Staff",
		"composite": false,
		"clientRole": false,
		"containerId": "realm-id",
		"attributes": {"permission": ["write", "read"]}
	}`)

	tests := []struct {
		name    string
		desired string
		want    bool
	}{
		{
			name:    "identical managed fields, extra Keycloak fields ignored",
			desired: `{"id":"526f","name":"hostzero-staff","description":"Staff"}`,
			want:    true,
		},
		{
			name:    "attributes compared as unordered string sets",
			desired: `{"id":"526f","name":"hostzero-staff","attributes":{"permission":["read","write"]}}`,
			want:    true,
		},
		{
			name:    "description differs",
			desired: `{"id":"526f","name":"hostzero-staff","description":"Changed"}`,
			want:    false,
		},
		{
			name:    "attribute value differs",
			desired: `{"id":"526f","name":"hostzero-staff","attributes":{"permission":["read"]}}`,
			want:    false,
		},
		{
			name:    "attribute missing in Keycloak",
			desired: `{"id":"526f","name":"hostzero-staff","attributes":{"tier":["gold"]}}`,
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := definitionsMatch(json.RawMessage(tt.desired), current); got != tt.want {
				t.Errorf("definitionsMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
