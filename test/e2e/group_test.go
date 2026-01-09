package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakGroupE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "group")

	t.Run("BasicGroup", func(t *testing.T) {
		groupName := fmt.Sprintf("test-group-%d", time.Now().UnixNano())
		groupDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, groupName))

		group := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: groupDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, group))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, group)
		})

		// Wait for group to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      group.Name,
				Namespace: group.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Group did not become ready")
		t.Logf("Group %s is ready", groupName)

		// Verify status
		updated := &keycloakv1beta1.KeycloakGroup{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      group.Name,
			Namespace: group.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.GroupID, "Group ID should be set")
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Group ID: %s, Resource path: %s", updated.Status.GroupID, updated.Status.ResourcePath)
	})

	t.Run("GroupWithAttributes", func(t *testing.T) {
		groupName := fmt.Sprintf("group-attrs-%d", time.Now().UnixNano())
		groupDef := rawJSON(fmt.Sprintf(`{
			"name": "%s",
			"attributes": {
				"department": ["engineering"],
				"location": ["remote"]
			}
		}`, groupName))

		group := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: groupDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, group))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, group)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      group.Name,
				Namespace: group.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Group with attributes did not become ready")
		t.Logf("Group with attributes %s is ready", groupName)
	})

	t.Run("NestedGroups", func(t *testing.T) {
		// Create parent group
		parentName := fmt.Sprintf("parent-group-%d", time.Now().UnixNano())
		parentDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, parentName))

		parentGroup := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      parentName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: parentDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, parentGroup))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, parentGroup)
		})

		// Wait for parent to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      parentGroup.Name,
				Namespace: parentGroup.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.GroupID != "", nil
		})
		require.NoError(t, err, "Parent group did not become ready")

		// Create child group
		childName := fmt.Sprintf("child-group-%d", time.Now().UnixNano())
		childDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, childName))

		childGroup := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      childName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:       &keycloakv1beta1.ResourceRef{Name: realmName},
				ParentGroupRef: &keycloakv1beta1.ResourceRef{Name: parentName},
				Definition:     childDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, childGroup))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, childGroup)
		})

		// Wait for child to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      childGroup.Name,
				Namespace: childGroup.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Child group did not become ready")
		t.Logf("Nested groups created: parent=%s, child=%s", parentName, childName)
	})

	t.Run("GroupCleanup", func(t *testing.T) {
		groupName := fmt.Sprintf("cleanup-group-%d", time.Now().UnixNano())
		groupDef := rawJSON(fmt.Sprintf(`{
			"name": "%s"
		}`, groupName))

		group := &keycloakv1beta1.KeycloakGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakGroupSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: groupDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, group))

		// Wait for ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakGroup{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      group.Name,
				Namespace: group.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Delete
		require.NoError(t, k8sClient.Delete(ctx, group))

		// Verify deleted from Kubernetes
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakGroup{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      group.Name,
				Namespace: group.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Group was not deleted")
		t.Logf("Group %s cleanup verified", groupName)
	})
}
