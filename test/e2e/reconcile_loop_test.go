package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TestNoSteadyStateStatusChurn asserts that reconciling an already-Ready,
// unchanged resource does not rewrite its status.
//
// A status write bumps resourceVersion, which fires the controller's own
// For() watch and re-enqueues the object immediately instead of honouring
// the sync period. The historical trigger was `LastTransitionTime:
// metav1.Now()` being refreshed on every pass: because metav1.Time has
// second granularity the loop only self-sustains when a reconcile crosses a
// second boundary, so asserting on loop *rate* is inherently flaky on a fast
// cluster. These sub-tests assert the underlying invariant instead — a no-op
// reconcile must leave lastTransitionTime and resourceVersion untouched.
func TestNoSteadyStateStatusChurn(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, _ := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, "noloop")

	// Covers the controller reported in #121 plus representatives of the
	// other status-write shapes: a plain namespaced child (group), one that
	// tracks ObservedGeneration (role), and one whose identity lives in a
	// sub-resource path (clientscope).
	cases := []struct {
		name string
		gvk  schema.GroupVersionKind
		make func(t *testing.T, name string) client.Object
	}{
		{
			name: "KeycloakRequiredAction",
			gvk:  keycloakv1beta1.GroupVersion.WithKind("KeycloakRequiredAction"),
			make: func(t *testing.T, name string) client.Object {
				return &keycloakv1beta1.KeycloakRequiredAction{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
					Spec: keycloakv1beta1.KeycloakRequiredActionSpec{
						RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
						Alias:    strPtr("UPDATE_PASSWORD"),
						Definition: rawJSON(`{
							"name": "Update Password",
							"providerId": "UPDATE_PASSWORD",
							"enabled": true,
							"defaultAction": true,
							"priority": 20
						}`),
					},
				}
			},
		},
		{
			name: "KeycloakGroup",
			gvk:  keycloakv1beta1.GroupVersion.WithKind("KeycloakGroup"),
			make: func(t *testing.T, name string) client.Object {
				return &keycloakv1beta1.KeycloakGroup{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
					Spec: keycloakv1beta1.KeycloakGroupSpec{
						RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
						Name:       strPtr(name),
						Definition: rawJSON(`{}`),
					},
				}
			},
		},
		{
			name: "KeycloakRole",
			gvk:  keycloakv1beta1.GroupVersion.WithKind("KeycloakRole"),
			make: func(t *testing.T, name string) client.Object {
				return &keycloakv1beta1.KeycloakRole{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
					Spec: keycloakv1beta1.KeycloakRoleSpec{
						RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
						Name:       strPtr(name),
						Definition: rawJSON(`{"description": "noloop role"}`),
					},
				}
			},
		},
		{
			name: "KeycloakClientScope",
			gvk:  keycloakv1beta1.GroupVersion.WithKind("KeycloakClientScope"),
			make: func(t *testing.T, name string) client.Object {
				return &keycloakv1beta1.KeycloakClientScope{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
					Spec: keycloakv1beta1.KeycloakClientScopeSpec{
						RealmRef: &keycloakv1beta1.ResourceRef{Name: realmName},
						Name:     strPtr(name),
						Definition: rawJSON(`{
							"protocol": "openid-connect",
							"description": "noloop scope"
						}`),
					},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := fmt.Sprintf("noloop-%s-%d", shortKind(tc.gvk.Kind), time.Now().UnixNano())
			obj := tc.make(t, name)

			require.NoError(t, k8sClient.Create(ctx, obj))
			t.Cleanup(func() { k8sClient.Delete(ctx, obj) })

			key := types.NamespacedName{Name: name, Namespace: testNamespace}
			waitForUnstructuredReady(t, tc.gvk, key)

			// Let any create-time follow-up writes settle so the baseline is
			// a genuinely quiescent object.
			time.Sleep(3 * time.Second)

			before := getUnstructured(t, tc.gvk, key)
			ltBefore := readyLastTransitionTime(t, before)
			rvBefore := before.GetResourceVersion()

			// Sleep past a second boundary: with the LastTransitionTime bug
			// the next reconcile necessarily computes a different timestamp,
			// which is what makes this deterministic rather than racy.
			time.Sleep(2 * time.Second)

			// Force exactly one reconcile. Nothing about the desired state
			// changed, so the controller must not write status at all.
			bumpReconcile(t, before.DeepCopy())

			after := getUnstructured(t, tc.gvk, key)
			ltAfter := readyLastTransitionTime(t, after)

			require.Equal(t, ltBefore, ltAfter,
				"Ready condition lastTransitionTime changed on a no-op reconcile; "+
					"status is being rewritten every pass, which re-triggers the controller's "+
					"own watch and causes a reconcile hot loop (issue #121)")

			// resourceVersion may legitimately have advanced once for the
			// annotation patch itself, but must then stop moving. A sustained
			// loop keeps bumping it.
			rvSettled := getUnstructured(t, tc.gvk, key).GetResourceVersion()
			time.Sleep(15 * time.Second)
			rvLater := getUnstructured(t, tc.gvk, key).GetResourceVersion()

			require.Equal(t, rvSettled, rvLater,
				"resourceVersion advanced from %s to %s while the resource was untouched "+
					"(baseline before reconcile was %s); the controller is re-reconciling "+
					"itself instead of honouring the sync period",
				rvSettled, rvLater, rvBefore)
		})
	}
}

// shortKind trims the common Keycloak prefix and lowercases the result so
// generated object names are valid RFC 1123 subdomains.
func shortKind(kind string) string {
	return strings.ToLower(strings.TrimPrefix(kind, "Keycloak"))
}

func getUnstructured(t *testing.T, gvk schema.GroupVersionKind, key types.NamespacedName) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	require.NoError(t, k8sClient.Get(ctx, key, u))
	return u
}

func waitForUnstructuredReady(t *testing.T, gvk schema.GroupVersionKind, key types.NamespacedName) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		if err := k8sClient.Get(ctx, key, u); err != nil {
			return false, nil
		}
		ready, found, err := unstructured.NestedBool(u.Object, "status", "ready")
		if err != nil || !found {
			return false, nil
		}
		return ready, nil
	})
	require.NoError(t, err, "%s %s did not become ready", gvk.Kind, key)
}

// readyLastTransitionTime extracts status.conditions[type=Ready].lastTransitionTime.
func readyLastTransitionTime(t *testing.T, u *unstructured.Unstructured) string {
	t.Helper()
	conditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	require.NoError(t, err)
	require.True(t, found, "no status.conditions on %s/%s", u.GetKind(), u.GetName())

	for _, raw := range conditions {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if c["type"] == "Ready" {
			lt, _ := c["lastTransitionTime"].(string)
			require.NotEmpty(t, lt, "Ready condition has no lastTransitionTime")
			return lt
		}
	}
	t.Fatalf("no Ready condition found on %s/%s", u.GetKind(), u.GetName())
	return ""
}
