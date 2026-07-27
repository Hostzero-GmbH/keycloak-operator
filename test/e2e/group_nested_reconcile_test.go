package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// TestKeycloakGroupNestedReconcile exercises the controller code path that
// looks up nested groups via /groups/{parentID}/children. Since Keycloak 23+
// the realm-wide /groups response no longer inlines subGroups, so the old
// recursive findGroupByNameInList walk would return nil for every child
// lookup and the reconciler would attempt to re-create a duplicate (Keycloak
// answers with 409) on every reconcile after the initial creation.
//
// These tests are integration tests against a real Keycloak (via the operator
// running in a kind cluster). They require:
//   - A working in-cluster operator pointed at a Keycloak >= 23
//   - Direct Keycloak API access from the test environment (port-forward)
func TestKeycloakGroupNestedReconcile(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "nested-reconcile")
	kc := getInternalKeycloakClient(t)

	// ChildSurvivesReReconcile is the most direct regression test for the
	// controller fix. Before the fix, the second reconcile of a nested child
	// would hit the buggy lookup, decide the group did not exist, and try to
	// create a duplicate — Keycloak returns 409 and the CR ends up in
	// CreateFailed.
	t.Run("ChildSurvivesReReconcile", func(t *testing.T) {
		suffix := time.Now().UnixNano()
		parentKCName := fmt.Sprintf("nrc-parent-%d", suffix)
		childKCName := fmt.Sprintf("nrc-child-%d", suffix)

		parent := newGroupCR(t, fmt.Sprintf("p-%d", suffix), realmName, "", parentKCName, nil)
		require.NoError(t, k8sClient.Create(ctx, parent))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, parent) })
		parentID := waitGroupReadyAndGetID(t, parent)

		child := newGroupCR(t, fmt.Sprintf("c-%d", suffix), realmName, parent.Name, childKCName, nil)
		require.NoError(t, k8sClient.Create(ctx, child))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, child) })
		childID := waitGroupReadyAndGetID(t, child)

		assertParentHasChildIDs(t, kc, realmName, parentID, []string{childID})

		// Force a re-reconcile by adding an attribute. Without the controller
		// fix the second reconcile would call CreateChildGroup again and
		// Keycloak would reject it with 409, dropping the CR into
		// CreateFailed. With the fix, the lookup finds the existing child
		// and the operator calls UpdateGroup instead.
		updateGroupDefinition(t, child, childKCName, map[string][]string{
			"reconciled": {"true"},
		})

		waitGroupAttributeApplied(t, kc, realmName, childID, "reconciled", "true")
		requireGroupStillReady(t, child, childID)
		assertParentHasChildIDs(t, kc, realmName, parentID, []string{childID})
	})

	// SameNameChildrenUnderDifferentParents proves that the new lookup is
	// correctly scoped to the parent's /children endpoint. The pre-fix
	// recursive findGroupByNameInList walked the realm-wide response and
	// could return any group with a matching name, regardless of parent.
	t.Run("SameNameChildrenUnderDifferentParents", func(t *testing.T) {
		suffix := time.Now().UnixNano()
		sharedKCName := fmt.Sprintf("nrc-shared-%d", suffix)

		parentA := newGroupCR(t, fmt.Sprintf("pa-%d", suffix), realmName, "", fmt.Sprintf("nrc-pa-%d", suffix), nil)
		require.NoError(t, k8sClient.Create(ctx, parentA))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, parentA) })
		parentAID := waitGroupReadyAndGetID(t, parentA)

		parentB := newGroupCR(t, fmt.Sprintf("pb-%d", suffix), realmName, "", fmt.Sprintf("nrc-pb-%d", suffix), nil)
		require.NoError(t, k8sClient.Create(ctx, parentB))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, parentB) })
		parentBID := waitGroupReadyAndGetID(t, parentB)

		childA := newGroupCR(t, fmt.Sprintf("ca-%d", suffix), realmName, parentA.Name, sharedKCName, nil)
		require.NoError(t, k8sClient.Create(ctx, childA))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, childA) })
		childAID := waitGroupReadyAndGetID(t, childA)

		childB := newGroupCR(t, fmt.Sprintf("cb-%d", suffix), realmName, parentB.Name, sharedKCName, nil)
		require.NoError(t, k8sClient.Create(ctx, childB))
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, childB) })
		childBID := waitGroupReadyAndGetID(t, childB)

		require.NotEqual(t, childAID, childBID, "same-name children under different parents must be distinct Keycloak groups")
		assertParentHasChildIDs(t, kc, realmName, parentAID, []string{childAID})
		assertParentHasChildIDs(t, kc, realmName, parentBID, []string{childBID})

		// Re-reconcile childA. The controller must scope the lookup to
		// parentA's children and update childA — not childB, which has the
		// same Keycloak group name but lives under parentB.
		updateGroupDefinition(t, childA, sharedKCName, map[string][]string{
			"side": {"a"},
		})

		waitGroupAttributeApplied(t, kc, realmName, childAID, "side", "a")
		requireGroupStillReady(t, childA, childAID)

		// childB must still exist, must not have been touched, and must
		// remain under parentB.
		rawB, err := kc.GetGroupRaw(ctx, realmName, childBID)
		require.NoError(t, err)
		var parsedB struct {
			Attributes map[string][]string `json:"attributes"`
		}
		require.NoError(t, json.Unmarshal(rawB, &parsedB))
		require.Empty(t, parsedB.Attributes["side"], "childB must not have been updated when reconciling childA")

		assertParentHasChildIDs(t, kc, realmName, parentAID, []string{childAID})
		assertParentHasChildIDs(t, kc, realmName, parentBID, []string{childBID})
	})
}

// newGroupCR builds a KeycloakGroup CR with the given Keycloak group name
// (set inside the definition) and an optional parent K8s CR name.
func newGroupCR(t *testing.T, k8sName, realmName, parentK8sName, kcGroupName string, attrs map[string][]string) *keycloakv1beta1.KeycloakGroup {
	t.Helper()

	body := map[string]interface{}{}
	if len(attrs) > 0 {
		body["attributes"] = attrs
	}
	def, err := json.Marshal(body)
	require.NoError(t, err)

	g := &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: testNamespace,
		},
		Spec: keycloakv1beta1.KeycloakGroupSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
			Name:       strPtr(kcGroupName),
			Definition: rawJSON(string(def)),
		},
	}
	if parentK8sName != "" {
		g.Spec.ParentGroupRef = &keycloakv1beta1.ResourceRef{Name: parentK8sName}
	}
	return g
}

func waitGroupReadyAndGetID(t *testing.T, group *keycloakv1beta1.KeycloakGroup) string {
	t.Helper()
	var groupID string
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakGroup{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      group.Name,
			Namespace: group.Namespace,
		}, updated); err != nil {
			return false, nil
		}
		if !updated.Status.Ready || updated.Status.GroupID == "" {
			return false, nil
		}
		groupID = updated.Status.GroupID
		return true, nil
	})
	require.NoErrorf(t, err, "KeycloakGroup %s did not become ready (status=%s)", group.Name, currentGroupStatus(group))
	return groupID
}

// updateGroupDefinition fetches the latest copy of the CR, replaces its
// Definition, and pushes the update — which triggers a re-reconcile.
func updateGroupDefinition(t *testing.T, group *keycloakv1beta1.KeycloakGroup, kcName string, attrs map[string][]string) {
	t.Helper()

	body := map[string]interface{}{
		"attributes": attrs,
	}
	def, err := json.Marshal(body)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		current := &keycloakv1beta1.KeycloakGroup{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      group.Name,
			Namespace: group.Namespace,
		}, current); err != nil {
			return false
		}
		current.Spec.Definition = rawJSON(string(def))
		return k8sClient.Update(ctx, current) == nil
	}, timeout, interval, "failed to update KeycloakGroup %s", group.Name)
}

// waitGroupAttributeApplied polls Keycloak until the group has the given
// attribute value applied, proving that the operator successfully called
// UpdateGroup on the existing nested child.
func waitGroupAttributeApplied(t *testing.T, kc *keycloak.Client, realmName, groupID, attrKey, want string) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		raw, err := kc.GetGroupRaw(ctx, realmName, groupID)
		if err != nil {
			return false
		}
		var parsed struct {
			Attributes map[string][]string `json:"attributes"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return false
		}
		vs := parsed.Attributes[attrKey]
		return len(vs) == 1 && vs[0] == want
	}, timeout, interval, "group %s never received attribute %s=%s in Keycloak", groupID, attrKey, want)
}

// requireGroupStillReady asserts the CR remained Ready (i.e. the re-reconcile
// did not drop into CreateFailed) and the GroupID was not changed.
func requireGroupStillReady(t *testing.T, group *keycloakv1beta1.KeycloakGroup, expectedID string) {
	t.Helper()
	final := &keycloakv1beta1.KeycloakGroup{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
		Name:      group.Name,
		Namespace: group.Namespace,
	}, final))
	require.Truef(t, final.Status.Ready,
		"KeycloakGroup %s should remain Ready after re-reconcile, got status=%q message=%q",
		group.Name, final.Status.Status, final.Status.Message)
	require.Equal(t, expectedID, final.Status.GroupID, "GroupID must remain stable across re-reconcile")
}

// assertParentHasChildIDs verifies that the given parent's children in
// Keycloak are exactly the expected set of group IDs (no missing, no
// duplicates).
func assertParentHasChildIDs(t *testing.T, kc *keycloak.Client, realmName, parentID string, wantIDs []string) {
	t.Helper()
	got, err := kc.GetGroupChildren(ctx, realmName, parentID, nil)
	require.NoError(t, err)

	gotIDs := make([]string, 0, len(got))
	for i := range got {
		if got[i].ID == nil {
			continue
		}
		gotIDs = append(gotIDs, *got[i].ID)
	}
	require.ElementsMatchf(t, wantIDs, gotIDs,
		"parent %s children mismatch (expected %v, got %v)", parentID, wantIDs, gotIDs)
}

// currentGroupStatus returns a small string describing the latest observed
// status of the CR — only used for failure messages.
func currentGroupStatus(group *keycloakv1beta1.KeycloakGroup) string {
	got := &keycloakv1beta1.KeycloakGroup{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      group.Name,
		Namespace: group.Namespace,
	}, got); err != nil {
		return fmt.Sprintf("get failed: %v", err)
	}
	return fmt.Sprintf("ready=%t status=%q message=%q groupID=%s",
		got.Status.Ready, got.Status.Status, got.Status.Message, got.Status.GroupID)
}
