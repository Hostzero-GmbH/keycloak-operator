package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

func TestFilterTopLevelExecutions(t *testing.T) {
	zero := 0
	one := 1

	tests := []struct {
		name     string
		input    []keycloak.AuthenticationExecutionInfo
		expected int
	}{
		{
			name:     "empty list",
			input:    nil,
			expected: 0,
		},
		{
			name: "all level 0",
			input: []keycloak.AuthenticationExecutionInfo{
				{Level: &zero},
				{Level: &zero},
			},
			expected: 2,
		},
		{
			name: "mixed levels",
			input: []keycloak.AuthenticationExecutionInfo{
				{Level: &zero},
				{Level: &one},
				{Level: &zero},
				{Level: &one},
			},
			expected: 2,
		},
		{
			name: "no level info returns all",
			input: []keycloak.AuthenticationExecutionInfo{
				{},
				{},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTopLevelExecutions(tt.input)
			require.Len(t, result, tt.expected)
		})
	}
}

func TestMatchesIdentifier(t *testing.T) {
	trueVal := true
	falseVal := false
	cookieProvider := "auth-cookie"
	formsAlias := "my-forms"

	tests := []struct {
		name     string
		exec     keycloak.AuthenticationExecutionInfo
		id       execIdentifier
		expected bool
	}{
		{
			name: "matches authenticator by provider ID",
			exec: keycloak.AuthenticationExecutionInfo{
				ProviderID:         &cookieProvider,
				AuthenticationFlow: &falseVal,
			},
			id:       execIdentifier{name: "auth-cookie", isFlow: false},
			expected: true,
		},
		{
			name: "does not match different provider",
			exec: keycloak.AuthenticationExecutionInfo{
				ProviderID:         &cookieProvider,
				AuthenticationFlow: &falseVal,
			},
			id:       execIdentifier{name: "auth-otp", isFlow: false},
			expected: false,
		},
		{
			name: "matches sub-flow by display name",
			exec: keycloak.AuthenticationExecutionInfo{
				DisplayName:        &formsAlias,
				AuthenticationFlow: &trueVal,
			},
			id:       execIdentifier{name: "my-forms", isFlow: true},
			expected: true,
		},
		{
			name: "does not match flow when looking for authenticator",
			exec: keycloak.AuthenticationExecutionInfo{
				DisplayName:        &formsAlias,
				AuthenticationFlow: &trueVal,
			},
			id:       execIdentifier{name: "my-forms", isFlow: false},
			expected: false,
		},
		{
			name: "does not match authenticator when looking for flow",
			exec: keycloak.AuthenticationExecutionInfo{
				ProviderID:         &cookieProvider,
				AuthenticationFlow: &falseVal,
			},
			id:       execIdentifier{name: "auth-cookie", isFlow: true},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesIdentifier(tt.exec, tt.id)
			require.Equal(t, tt.expected, result)
		})
	}
}
