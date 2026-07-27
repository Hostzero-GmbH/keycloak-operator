package controller

import "testing"

func strptr(s string) *string { return &s }

func TestResolveIdentifier(t *testing.T) {
	tests := []struct {
		name         string
		specVal      *string
		defVal       string
		wantResolved string
		wantErr      bool
	}{
		{
			name:         "spec set, no in-definition value",
			specVal:      strptr("from-spec"),
			defVal:       "",
			wantResolved: "from-spec",
			wantErr:      false,
		},
		{
			name:    "spec unset is rejected (required)",
			specVal: nil,
			defVal:  "",
			wantErr: true,
		},
		{
			name:    "empty spec pointer is rejected",
			specVal: strptr(""),
			defVal:  "",
			wantErr: true,
		},
		{
			name:    "in-definition identifier without a spec value is rejected",
			specVal: nil,
			defVal:  "from-def",
			wantErr: true,
		},
		{
			name:         "in-definition identifier matching spec is accepted",
			specVal:      strptr("same"),
			defVal:       "same",
			wantResolved: "same",
			wantErr:      false,
		},
		{
			name:    "in-definition identifier conflicting with spec is rejected",
			specVal: strptr("from-spec"),
			defVal:  "from-def",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveIdentifier("name", tt.specVal, tt.defVal)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got resolved=%q, nil error", resolved)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %q, want %q", resolved, tt.wantResolved)
			}
		})
	}
}

func TestIdentifierValue(t *testing.T) {
	if got := identifierValue(nil); got != "" {
		t.Errorf("identifierValue(nil) = %q, want empty", got)
	}
	if got := identifierValue(strptr("x")); got != "x" {
		t.Errorf("identifierValue(\"x\") = %q, want \"x\"", got)
	}
}
