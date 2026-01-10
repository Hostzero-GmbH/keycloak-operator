package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestKeycloakOrganizationE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)

	// First verify the instance version is >= 26 (organizations require Keycloak 26+)
	instance := &keycloakv1beta1.KeycloakInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: instanceNS}, instance)
	require.NoError(t, err)
	t.Logf("Keycloak version: %s", instance.Status.Version)

	// Organizations require Keycloak 26+
	if instance.Status.Version == "" || instance.Status.Version[0:2] < "26" {
		t.Skip("Organizations require Keycloak 26.0.0 or later")
	}

	// Create a realm with organizations enabled (required for Keycloak 26+)
	realmName := createTestRealmWithOrganizations(t, instanceName, instanceNS, "organization")

	t.Run("BasicOrganization", func(t *testing.T) {
		orgName := fmt.Sprintf("test-org-%d", time.Now().UnixNano())
		// Keycloak 26+ requires at least one domain for organizations
		orgDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"alias": "%s",
			"description": "Test organization for E2E",
			"enabled": true,
			"domains": [{"name": "%s.example.com", "verified": false}]
		}`, orgName, orgName, orgName))
		kcOrg := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: orgDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcOrg))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcOrg)
		})

		// Wait for organization to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakOrganization{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcOrg.Name,
				Namespace: kcOrg.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakOrganization did not become ready")

		// Verify status fields
		updatedOrg := &keycloakv1beta1.KeycloakOrganization{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcOrg.Name,
			Namespace: kcOrg.Namespace,
		}, updatedOrg)
		require.NoError(t, err)
		require.NotEmpty(t, updatedOrg.Status.OrganizationID, "OrganizationID should be set")
		require.Contains(t, updatedOrg.Status.ResourcePath, "/organizations/", "ResourcePath should contain /organizations/")
		t.Logf("KeycloakOrganization %s is ready, ID=%s", orgName, updatedOrg.Status.OrganizationID)
	})

	t.Run("OrganizationWithDomains", func(t *testing.T) {
		orgName := fmt.Sprintf("test-org-domains-%d", time.Now().UnixNano())
		orgDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"alias": "%s",
			"description": "Organization with domains",
			"enabled": true,
			"domains": [
				{"name": "example.com", "verified": false},
				{"name": "test.org", "verified": false}
			]
		}`, orgName, orgName))
		kcOrg := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: orgDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcOrg))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcOrg)
		})

		// Wait for organization to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakOrganization{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcOrg.Name,
				Namespace: kcOrg.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakOrganization with domains did not become ready")
		t.Logf("KeycloakOrganization with domains %s is ready", orgName)
	})

	t.Run("OrganizationUpdate", func(t *testing.T) {
		orgName := fmt.Sprintf("test-org-update-%d", time.Now().UnixNano())
		// Keycloak 26+ requires at least one domain for organizations
		orgDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"alias": "%s",
			"description": "Original description",
			"enabled": true,
			"domains": [{"name": "%s.example.com", "verified": false}]
		}`, orgName, orgName, orgName))
		kcOrg := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: orgDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcOrg))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcOrg)
		})

		// Wait for organization to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakOrganization{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcOrg.Name,
				Namespace: kcOrg.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Initial organization did not become ready")

		// Update the organization
		updatedOrg := &keycloakv1beta1.KeycloakOrganization{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcOrg.Name,
			Namespace: kcOrg.Namespace,
		}, updatedOrg)
		require.NoError(t, err)

		newDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"alias": "%s",
			"description": "Updated description",
			"enabled": true,
			"domains": [{"name": "%s.example.com", "verified": false}]
		}`, orgName, orgName, orgName))
		updatedOrg.Spec.Definition = newDef
		require.NoError(t, k8sClient.Update(ctx, updatedOrg))

		// Wait for reconciliation
		time.Sleep(2 * time.Second)

		// Verify it's still ready
		finalOrg := &keycloakv1beta1.KeycloakOrganization{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcOrg.Name,
			Namespace: kcOrg.Namespace,
		}, finalOrg)
		require.NoError(t, err)
		require.True(t, finalOrg.Status.Ready, "Organization should still be ready after update")
		t.Logf("KeycloakOrganization %s updated successfully", orgName)
	})

	t.Run("InvalidRealmRef", func(t *testing.T) {
		orgName := fmt.Sprintf("invalid-realm-org-%d", time.Now().UnixNano())
		orgDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"enabled": true
		}`, orgName))
		kcOrg := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: "non-existent-realm"},
				Definition: orgDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcOrg))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcOrg)
		})

		// Wait and verify the org is NOT ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakOrganization{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      orgName,
			Namespace: testNamespace,
		}, updated)
		require.NoError(t, err)
		require.False(t, updated.Status.Ready, "Organization with invalid realm ref should not be ready")
		t.Logf("Organization correctly failed with invalid realm ref, message: %s", updated.Status.Message)
	})
}
