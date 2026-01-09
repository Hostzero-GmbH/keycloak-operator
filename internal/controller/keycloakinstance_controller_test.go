package controller

import (
	"testing"
)

func TestValidateKeycloakVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		// Supported versions
		{name: "version 20.0.0", version: "20.0.0", wantErr: false},
		{name: "version 20.0.1", version: "20.0.1", wantErr: false},
		{name: "version 21.0.0", version: "21.0.0", wantErr: false},
		{name: "version 22.0.0", version: "22.0.0", wantErr: false},
		{name: "version 23.0.0", version: "23.0.0", wantErr: false},
		{name: "version 24.0.0", version: "24.0.0", wantErr: false},
		{name: "version 25.0.0", version: "25.0.0", wantErr: false},
		{name: "version 26.0.0", version: "26.0.0", wantErr: false},
		{name: "version with snapshot", version: "24.0.0-SNAPSHOT", wantErr: false},
		{name: "version with RC", version: "25.0.0-RC1", wantErr: false},

		// Unsupported versions
		{name: "version 19.0.0", version: "19.0.0", wantErr: true},
		{name: "version 18.0.0", version: "18.0.0", wantErr: true},
		{name: "version 17.0.0", version: "17.0.0", wantErr: true},
		{name: "version 4.0.0 (ancient)", version: "4.0.0", wantErr: true},
		{name: "version 19 with snapshot", version: "19.0.0-SNAPSHOT", wantErr: true},

		// Edge cases
		{name: "major version only", version: "20", wantErr: false},
		{name: "major.minor only", version: "21.1", wantErr: false},
		{name: "unsupported major only", version: "19", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeycloakVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKeycloakVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}
