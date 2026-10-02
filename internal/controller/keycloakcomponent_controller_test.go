package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
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

func TestFindMatchingComponentIDRequiresParentID(t *testing.T) {
	firstID, secondID := "mapper-under-ldap-a", "mapper-under-ldap-b"
	name := "email"
	providerID := "user-attribute-ldap-mapper"
	providerType := "org.keycloak.storage.ldap.mappers.LDAPStorageMapper"
	parentA, parentB := "ldap-a", "ldap-b"

	components := []keycloak.ComponentRepresentation{
		{ID: &firstID, Name: &name, ProviderID: &providerID, ProviderType: &providerType, ParentID: &parentA},
		{ID: &secondID, Name: &name, ProviderID: &providerID, ProviderType: &providerType, ParentID: &parentB},
	}
	desired := componentIdentity{Name: name, ProviderID: providerID, ProviderType: providerType}

	desired.ParentID = parentB
	got, err := findMatchingComponentID(components, desired)
	require.NoError(t, err)
	require.Equal(t, secondID, got)

	desired.ParentID = "ldap-c"
	got, err = findMatchingComponentID(components, desired)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestGetReadyParentComponent(t *testing.T) {
	ready := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "ldap", Namespace: "ns"},
		Status:     keycloakv1beta1.KeycloakComponentStatus{Ready: true, ComponentID: "ldap-id"},
	}
	notReady := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "ns"},
		Status:     keycloakv1beta1.KeycloakComponentStatus{Ready: false, ComponentID: "pending-id"},
	}
	noID := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "no-id", Namespace: "ns"},
		Status:     keycloakv1beta1.KeycloakComponentStatus{Ready: true},
	}
	cl := fake.NewClientBuilder().WithScheme(configSecretScheme(t)).WithObjects(ready, notReady, noID).Build()

	child := func(parent string) *keycloakv1beta1.KeycloakComponent {
		return &keycloakv1beta1.KeycloakComponent{
			ObjectMeta: metav1.ObjectMeta{Name: "mapper", Namespace: "ns"},
			Spec:       keycloakv1beta1.KeycloakComponentSpec{ParentComponentRef: &keycloakv1beta1.ResourceRef{Name: parent}},
		}
	}

	got, err := getReadyParentComponent(context.Background(), cl, child("ldap"))
	require.NoError(t, err)
	require.Equal(t, "ldap-id", got.Status.ComponentID)

	for _, name := range []string{"missing", "pending", "no-id"} {
		_, err := getReadyParentComponent(context.Background(), cl, child(name))
		require.Error(t, err, name)
	}
}

func TestSameRealmRef(t *testing.T) {
	realm := func(name string) *keycloakv1beta1.KeycloakComponent {
		return &keycloakv1beta1.KeycloakComponent{Spec: keycloakv1beta1.KeycloakComponentSpec{RealmRef: &keycloakv1beta1.ResourceRef{Name: name}}}
	}
	clusterRealm := func(name string) *keycloakv1beta1.KeycloakComponent {
		return &keycloakv1beta1.KeycloakComponent{Spec: keycloakv1beta1.KeycloakComponentSpec{ClusterRealmRef: &keycloakv1beta1.ClusterResourceRef{Name: name}}}
	}

	require.True(t, sameRealmRef(realm("a"), realm("a")))
	require.True(t, sameRealmRef(clusterRealm("a"), clusterRealm("a")))
	require.False(t, sameRealmRef(realm("a"), realm("b")))
	require.False(t, sameRealmRef(realm("a"), clusterRealm("a")))
}

func TestFindComponentsForParent(t *testing.T) {
	parent := &keycloakv1beta1.KeycloakComponent{ObjectMeta: metav1.ObjectMeta{Name: "ldap", Namespace: "ns"}}
	childOf := func(name, namespace, parentName string) *keycloakv1beta1.KeycloakComponent {
		c := &keycloakv1beta1.KeycloakComponent{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		if parentName != "" {
			c.Spec.ParentComponentRef = &keycloakv1beta1.ResourceRef{Name: parentName}
		}
		return c
	}
	cl := fake.NewClientBuilder().WithScheme(configSecretScheme(t)).WithObjects(
		parent,
		childOf("mapper", "ns", "ldap"),
		childOf("other-mapper", "ns", "other-ldap"),
		childOf("top-level", "ns", ""),
		childOf("mapper", "other-ns", "ldap"),
	).Build()

	r := &KeycloakComponentReconciler{Client: cl}
	reqs := r.findComponentsForParent(context.Background(), parent)
	require.Equal(t, []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "mapper", Namespace: "ns"}}}, reqs)
}
