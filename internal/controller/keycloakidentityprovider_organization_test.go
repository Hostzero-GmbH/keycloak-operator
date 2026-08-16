package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestSameRealmPlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		idp  keycloakv1beta1.KeycloakIdentityProviderSpec
		org  keycloakv1beta1.KeycloakOrganizationSpec
		want bool
	}{
		{
			name: "matching realmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			want: true,
		},
		{
			name: "mismatched realmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-a"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "realm-b"}},
			want: false,
		},
		{
			name: "matching clusterRealmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			want: true,
		},
		{
			name: "realmRef vs clusterRealmRef",
			idp:  keycloakv1beta1.KeycloakIdentityProviderSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: "shared"}},
			org:  keycloakv1beta1.KeycloakOrganizationSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: "shared"}},
			want: false,
		},
		{
			name: "both unset",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idp := &keycloakv1beta1.KeycloakIdentityProvider{Spec: tc.idp}
			org := &keycloakv1beta1.KeycloakOrganization{Spec: tc.org}
			require.Equal(t, tc.want, sameRealmPlacement(idp, org))
		})
	}
}

func TestIsOrganizationRealmMismatch(t *testing.T) {
	t.Parallel()
	require.True(t, isOrganizationRealmMismatch(organizationRealmMismatchError{}))
	require.False(t, isOrganizationRealmMismatch(nil))
}
