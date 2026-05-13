package controller

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

func TestStringSetsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "both nil", a: nil, b: nil, want: true},
		{name: "both empty", a: []string{}, b: []string{}, want: true},
		{name: "nil vs empty", a: nil, b: []string{}, want: true},
		{name: "single equal", a: []string{"x"}, b: []string{"x"}, want: true},
		{name: "order-insensitive", a: []string{"a", "b", "c"}, b: []string{"c", "a", "b"}, want: true},
		{name: "length differs", a: []string{"a", "b"}, b: []string{"a"}, want: false},
		{name: "different members", a: []string{"a", "b"}, b: []string{"a", "c"}, want: false},
		{name: "duplicates collapse on left", a: []string{"a", "a"}, b: []string{"a"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, stringSetsEqual(tc.a, tc.b))
			// symmetric
			require.Equal(t, tc.want, stringSetsEqual(tc.b, tc.a))
		})
	}
}

func TestPermissionPoliciesMatch(t *testing.T) {
	// Thin wrapper over stringSetsEqual; just smoke-test the indirection so
	// the symbol stays callable / typed correctly.
	require.True(t, permissionPoliciesMatch(nil, nil))
	require.True(t, permissionPoliciesMatch([]string{"p1"}, []string{"p1"}))
	require.False(t, permissionPoliciesMatch([]string{"p1"}, []string{"p2"}))
	require.False(t, permissionPoliciesMatch([]string{"p1", "p2"}, []string{"p1"}))
}

func TestClientPolicyEqual(t *testing.T) {
	base := func() *keycloak.ClientPolicyRepresentation {
		return &keycloak.ClientPolicyRepresentation{
			Name:             "p",
			Description:      "d",
			Type:             "client",
			Logic:            "POSITIVE",
			DecisionStrategy: "UNANIMOUS",
			Clients:          []string{"uuid-a", "uuid-b"},
		}
	}

	t.Run("identical", func(t *testing.T) {
		require.True(t, clientPolicyEqual(base(), base()))
	})

	t.Run("client set order-insensitive", func(t *testing.T) {
		a := base()
		b := base()
		b.Clients = []string{"uuid-b", "uuid-a"}
		require.True(t, clientPolicyEqual(a, b))
	})

	t.Run("name differs", func(t *testing.T) {
		a := base()
		b := base()
		b.Name = "other"
		require.False(t, clientPolicyEqual(a, b))
	})

	t.Run("description differs", func(t *testing.T) {
		a := base()
		b := base()
		b.Description = "other"
		require.False(t, clientPolicyEqual(a, b))
	})

	t.Run("logic differs", func(t *testing.T) {
		a := base()
		b := base()
		b.Logic = "NEGATIVE"
		require.False(t, clientPolicyEqual(a, b))
	})

	t.Run("decisionStrategy differs", func(t *testing.T) {
		a := base()
		b := base()
		b.DecisionStrategy = "AFFIRMATIVE"
		require.False(t, clientPolicyEqual(a, b))
	})

	t.Run("client set differs", func(t *testing.T) {
		a := base()
		b := base()
		b.Clients = []string{"uuid-a"}
		require.False(t, clientPolicyEqual(a, b))
	})

	t.Run("ID is ignored", func(t *testing.T) {
		// The remote ID is set by Keycloak after creation; the operator
		// must not treat the existing policy as drifted just because the
		// desired representation hasn't been assigned an ID yet.
		a := base()
		a.ID = "remote-side-uuid"
		b := base()
		require.True(t, clientPolicyEqual(a, b))
	})
}

func TestIsTokenExchangeWaiting(t *testing.T) {
	t.Run("nil is not waiting", func(t *testing.T) {
		require.False(t, IsTokenExchangeWaiting(nil))
	})
	t.Run("unrelated error is not waiting", func(t *testing.T) {
		require.False(t, IsTokenExchangeWaiting(errors.New("boom")))
	})
	t.Run("sentinel itself", func(t *testing.T) {
		require.True(t, IsTokenExchangeWaiting(errTokenExchangeWaiting))
	})
	t.Run("wrapped sentinel", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: client %q missing", errTokenExchangeWaiting, "foo")
		require.True(t, IsTokenExchangeWaiting(wrapped))
	})
	t.Run("double-wrapped sentinel", func(t *testing.T) {
		inner := fmt.Errorf("%w: x", errTokenExchangeWaiting)
		outer := fmt.Errorf("wrapping: %w", inner)
		require.True(t, IsTokenExchangeWaiting(outer))
	})
}
