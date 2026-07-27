package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

func TestFindMatchingComponentIDByNameAndProviderType(t *testing.T) {
	componentID := "named-component"
	componentName := "rsa-key"
	providerType := "org.keycloak.keys.KeyProvider"
	providerID := "rsa-generated"
	parentID := "realm-id"

	components := []keycloak.ComponentRepresentation{
		{
			ID:           &componentID,
			Name:         &componentName,
			ProviderID:   &providerID,
			ProviderType: &providerType,
			ParentID:     &parentID,
		},
	}

	got, err := findMatchingComponentID(components, componentIdentity{
		Name:         componentName,
		ProviderID:   providerID,
		ProviderType: providerType,
		ParentID:     parentID,
	})
	require.NoError(t, err)
	require.Equal(t, componentID, got)
}

func TestFindMatchingComponentIDAdoptsUnnamedUserProfileComponent(t *testing.T) {
	componentID := "existing-user-profile-component"
	providerID := declarativeUserProfileProviderID
	providerType := userProfileProviderType
	parentID := "realm-id"

	components := []keycloak.ComponentRepresentation{
		{
			ID:           &componentID,
			ProviderID:   &providerID,
			ProviderType: &providerType,
			ParentID:     &parentID,
			// Keycloak-created user-profile components can be unnamed when they
			// are created through the /users/profile Admin API or User Profile UI.
			Name: nil,
		},
	}

	got, err := findMatchingComponentID(components, componentIdentity{
		Name:         declarativeUserProfileProviderID,
		ProviderID:   declarativeUserProfileProviderID,
		ProviderType: userProfileProviderType,
		ParentID:     parentID,
	})
	require.NoError(t, err)
	require.Equal(t, componentID, got)
}

func TestFindMatchingComponentIDDoesNotFallbackForOtherComponents(t *testing.T) {
	componentID := "unnamed-rsa-component"
	providerID := "rsa-generated"
	providerType := "org.keycloak.keys.KeyProvider"
	parentID := "realm-id"

	components := []keycloak.ComponentRepresentation{
		{
			ID:           &componentID,
			ProviderID:   &providerID,
			ProviderType: &providerType,
			ParentID:     &parentID,
			Name:         nil,
		},
	}

	got, err := findMatchingComponentID(components, componentIdentity{
		Name:         "rsa-key",
		ProviderID:   providerID,
		ProviderType: providerType,
		ParentID:     parentID,
	})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestFindMatchingComponentIDReturnsErrorForAmbiguousUserProfileComponents(t *testing.T) {
	firstID := "first"
	secondID := "second"
	providerID := declarativeUserProfileProviderID
	providerType := userProfileProviderType
	parentID := "realm-id"

	components := []keycloak.ComponentRepresentation{
		{ID: &firstID, ProviderID: &providerID, ProviderType: &providerType, ParentID: &parentID},
		{ID: &secondID, ProviderID: &providerID, ProviderType: &providerType, ParentID: &parentID},
	}

	got, err := findMatchingComponentID(components, componentIdentity{
		Name:         declarativeUserProfileProviderID,
		ProviderID:   declarativeUserProfileProviderID,
		ProviderType: userProfileProviderType,
		ParentID:     parentID,
	})
	require.Error(t, err)
	require.Empty(t, got)
}
