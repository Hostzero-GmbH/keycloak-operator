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
