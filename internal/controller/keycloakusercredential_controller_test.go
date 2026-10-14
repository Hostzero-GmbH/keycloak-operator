package controller

import (
	"strings"
	"testing"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func boolptr(b bool) *bool { return &b }

func TestGeneratePasswordHonoursPolicy(t *testing.T) {
	tests := []struct {
		name          string
		policy        *keycloakv1beta1.PasswordPolicySpec
		wantLen       int
		wantNoNumbers bool
		wantNoSymbols bool
	}{
		{
			name:    "nil policy uses the documented defaults",
			policy:  nil,
			wantLen: 24,
		},
		{
			name:    "length is honoured",
			policy:  &keycloakv1beta1.PasswordPolicySpec{Length: 40},
			wantLen: 40,
		},
		{
			name:          "includeSymbols false excludes symbols",
			policy:        &keycloakv1beta1.PasswordPolicySpec{Length: 32, IncludeSymbols: boolptr(false)},
			wantLen:       32,
			wantNoSymbols: true,
		},
		{
			name:          "includeNumbers false excludes numbers",
			policy:        &keycloakv1beta1.PasswordPolicySpec{Length: 32, IncludeNumbers: boolptr(false)},
			wantLen:       32,
			wantNoNumbers: true,
		},
		{
			name: "both disabled leaves letters only",
			policy: &keycloakv1beta1.PasswordPolicySpec{
				Length:         32,
				IncludeNumbers: boolptr(false),
				IncludeSymbols: boolptr(false),
			},
			wantLen:       32,
			wantNoNumbers: true,
			wantNoSymbols: true,
		},
	}

	r := &KeycloakUserCredentialReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generated passwords are random, so a single sample can pass by
			// luck. Repeat enough that an ignored policy field is caught.
			for i := 0; i < 200; i++ {
				got, err := r.generatePassword(tt.policy)
				if err != nil {
					t.Fatalf("generatePassword returned error: %v", err)
				}
				if len(got) != tt.wantLen {
					t.Fatalf("length = %d, want %d (%q)", len(got), tt.wantLen, got)
				}
				if tt.wantNoNumbers && strings.ContainsAny(got, passwordNumbers) {
					t.Fatalf("password contains a number despite includeNumbers=false: %q", got)
				}
				if tt.wantNoSymbols && strings.ContainsAny(got, passwordSymbols) {
					t.Fatalf("password contains a symbol despite includeSymbols=false: %q", got)
				}
				for _, c := range got {
					if !strings.ContainsRune(passwordLetters+passwordNumbers+passwordSymbols, c) {
						t.Fatalf("password contains character %q outside the alphabet: %q", c, got)
					}
				}
			}
		})
	}
}

func TestGeneratePasswordUsesTheWholeAlphabet(t *testing.T) {
	// Guards against a generator that silently narrows the character set, which
	// is what the base64 implementation did: it could never emit most symbols.
	r := &KeycloakUserCredentialReconciler{}
	seen := map[rune]bool{}
	for i := 0; i < 500; i++ {
		got, err := r.generatePassword(&keycloakv1beta1.PasswordPolicySpec{Length: 64})
		if err != nil {
			t.Fatalf("generatePassword returned error: %v", err)
		}
		for _, c := range got {
			seen[c] = true
		}
	}
	for _, c := range passwordLetters + passwordNumbers + passwordSymbols {
		if !seen[c] {
			t.Errorf("character %q was never generated in 500 passwords of length 64", c)
		}
	}
}

func TestGeneratePasswordAlwaysSatisfiesEnabledClasses(t *testing.T) {
	// Keycloak rejects the password outright when the realm policy requires a
	// digit or a special character, and the credential then never becomes
	// ready. Drawing uniformly from the alphabet usually includes one of each
	// but not always, so the generator has to guarantee it.
	r := &KeycloakUserCredentialReconciler{}
	for i := 0; i < 2000; i++ {
		got, err := r.generatePassword(&keycloakv1beta1.PasswordPolicySpec{Length: 12})
		if err != nil {
			t.Fatalf("generatePassword returned error: %v", err)
		}
		if !strings.ContainsAny(got, passwordNumbers) {
			t.Fatalf("password has no digit with includeNumbers defaulted on: %q", got)
		}
		if !strings.ContainsAny(got, passwordSymbols) {
			t.Fatalf("password has no symbol with includeSymbols defaulted on: %q", got)
		}
		if !strings.ContainsAny(got, passwordLetters) {
			t.Fatalf("password has no letter: %q", got)
		}
	}
}

func TestGeneratePasswordShortLengthDoesNotPanic(t *testing.T) {
	// Length has no kubebuilder Minimum, so a 1 or 2 character password is
	// reachable from a valid CR and must not break the generator.
	r := &KeycloakUserCredentialReconciler{}
	for _, n := range []int{1, 2, 3} {
		got, err := r.generatePassword(&keycloakv1beta1.PasswordPolicySpec{Length: n})
		if err != nil {
			t.Fatalf("length %d returned error: %v", n, err)
		}
		if len(got) != n {
			t.Fatalf("length %d produced %d characters (%q)", n, len(got), got)
		}
	}
}
