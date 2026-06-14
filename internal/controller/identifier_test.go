package controller

import "testing"

func strptr(s string) *string { return &s }

func TestResolveIdentifier(t *testing.T) {
	tests := []struct {
		name         string
		specVal      *string
		defVal       string
		metaName     string
		wantResolved string
		wantMismatch bool
	}{
		{
			name:         "spec only",
			specVal:      strptr("from-spec"),
			defVal:       "",
			metaName:     "meta",
			wantResolved: "from-spec",
			wantMismatch: false,
		},
		{
			name:         "definition only falls through",
			specVal:      nil,
			defVal:       "from-def",
			metaName:     "meta",
			wantResolved: "from-def",
			wantMismatch: false,
		},
		{
			name:         "metadata.name fallback when neither set",
			specVal:      nil,
			defVal:       "",
			metaName:     "meta",
			wantResolved: "meta",
			wantMismatch: false,
		},
		{
			name:         "spec wins over definition when both set and agree",
			specVal:      strptr("same"),
			defVal:       "same",
			metaName:     "meta",
			wantResolved: "same",
			wantMismatch: false,
		},
		{
			name:         "mismatch: spec wins, mismatch reported",
			specVal:      strptr("from-spec"),
			defVal:       "from-def",
			metaName:     "meta",
			wantResolved: "from-spec",
			wantMismatch: true,
		},
		{
			name:         "empty spec pointer falls through to definition",
			specVal:      strptr(""),
			defVal:       "from-def",
			metaName:     "meta",
			wantResolved: "from-def",
			wantMismatch: false,
		},
		{
			name:         "empty spec pointer falls through to metadata.name",
			specVal:      strptr(""),
			defVal:       "",
			metaName:     "meta",
			wantResolved: "meta",
			wantMismatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, mismatch := resolveIdentifier(tt.specVal, tt.defVal, tt.metaName)
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %q, want %q", resolved, tt.wantResolved)
			}
			if mismatch != tt.wantMismatch {
				t.Errorf("mismatch = %v, want %v", mismatch, tt.wantMismatch)
			}
		})
	}
}
