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

// TestKeycloakRealmIdentifierUnification exercises the first-class identifier
// behavior for realms: the realmName-only regression, spec-over-definition
// precedence on mismatch, and the realmName immutability CEL rule.
func TestKeycloakRealmIdentifierUnification(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)

	// Regression for the historical symptom: a realm with ONLY spec.realmName set
	// and a definition that has NO realm key must become Ready and be created in
	// Keycloak under that name. Previously this failed with "Realm name is
	// required in definition".
	t.Run("RealmNameOnlyNoDefinitionRealm", func(t *testing.T) {
		crName := fmt.Sprintf("rn-only-%d", time.Now().UnixNano())
		realmInKeycloak := fmt.Sprintf("kc-rn-only-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(realmInKeycloak),
				// Deliberately no "realm" key in the definition.
				Definition: rawJSON(`{"enabled": true, "displayName": "Realm Name Only"}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		updated := waitRealmReady(t, crName)
		require.Equal(t, realmInKeycloak, updated.Status.RealmName,
			"status.realmName should reflect the resolved realm name")

		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			_, err := kc.GetRealm(ctx, realmInKeycloak)
			require.NoError(t, err, "realm should exist in Keycloak under spec.realmName")
		}
		t.Logf("realm %q created from spec.realmName only", realmInKeycloak)
	})

	// spec.realmName must win when it differs from definition.realm (Phase 1:
	// spec wins, reconcile continues, and a soft mismatch warning is logged).
	t.Run("SpecOverridesDefinitionRealmMismatch", func(t *testing.T) {
		crName := fmt.Sprintf("rn-mismatch-%d", time.Now().UnixNano())
		specRealm := fmt.Sprintf("kc-spec-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(specRealm),
				Definition:  rawJSON(`{"realm": "from-definition-should-be-overridden", "enabled": true}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		updated := waitRealmReady(t, crName)
		require.Equal(t, specRealm, updated.Status.RealmName,
			"spec.realmName must win over definition.realm")

		if canConnectToKeycloak() {
			kc := getInternalKeycloakClient(t)
			_, err := kc.GetRealm(ctx, specRealm)
			require.NoError(t, err, "realm should exist under spec.realmName, not definition.realm")
			_, err = kc.GetRealm(ctx, "from-definition-should-be-overridden")
			require.Error(t, err, "the definition.realm value must NOT have been used")
		}
	})

	// nil -> value migration is permitted; value -> different value is rejected
	// by the spec-level CEL transition rule.
	t.Run("RealmNameImmutableOnceSet", func(t *testing.T) {
		crName := fmt.Sprintf("rn-immutable-%d", time.Now().UnixNano())
		firstName := fmt.Sprintf("kc-immutable-%d", time.Now().UnixNano())

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:   strPtr(firstName),
				Definition:  rawJSON(`{"enabled": true}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		waitRealmReady(t, crName)

		// Attempt to change the immutable realmName -> must be rejected at apply time.
		current := &keycloakv1beta1.KeycloakRealm{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: testNamespace}, current))
		current.Spec.RealmName = strPtr(firstName + "-changed")
		err := k8sClient.Update(ctx, current)
		require.Error(t, err, "changing spec.realmName must be rejected by the immutability rule")
		require.Contains(t, err.Error(), "immutable", "rejection should cite immutability")
		t.Logf("realmName change correctly rejected: %v", err)
	})
}

func waitRealmReady(t *testing.T, name string) *keycloakv1beta1.KeycloakRealm {
	t.Helper()
	updated := &keycloakv1beta1.KeycloakRealm{}
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakRealm %s did not become ready", name)
	return updated
}

func strPtr(s string) *string { return &s }
