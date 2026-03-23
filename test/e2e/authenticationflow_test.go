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

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

func TestKeycloakAuthenticationFlowE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "authflow")

	t.Run("SimpleFlow", func(t *testing.T) {
		flowAlias := fmt.Sprintf("simple-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      flowAlias,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Simple test flow",
				ProviderId:  "basic-flow",
				Executions: []keycloakv1beta1.AuthenticationExecution{
					{
						Authenticator: "auth-cookie",
						Requirement:   "ALTERNATIVE",
					},
					{
						Authenticator: "auth-spnego",
						Requirement:   "DISABLED",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, flow)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Authentication flow did not become ready")

		updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      flow.Name,
			Namespace: flow.Namespace,
		}, updated))
		require.NotEmpty(t, updated.Status.FlowID, "Flow ID should be set")
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Flow %s is ready with ID %s", flowAlias, updated.Status.FlowID)
	})

	t.Run("FlowWithSubFlows", func(t *testing.T) {
		flowAlias := fmt.Sprintf("nested-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      flowAlias,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Nested test flow",
				ProviderId:  "basic-flow",
				Executions: []keycloakv1beta1.AuthenticationExecution{
					{
						Authenticator: "auth-cookie",
						Requirement:   "ALTERNATIVE",
					},
					{
						SubFlow: &keycloakv1beta1.SubFlowDefinition{
							Alias:       flowAlias + "-forms",
							Description: "Form sub-flow",
							ProviderId:  "basic-flow",
							Executions: []keycloakv1beta1.SubFlowExecution{
								{
									Authenticator: "auth-username-password-form",
									Requirement:   "REQUIRED",
								},
							},
						},
						Requirement: "ALTERNATIVE",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, flow)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Nested flow did not become ready")
		t.Logf("Nested flow %s is ready", flowAlias)
	})

	t.Run("FlowWithKeycloakVerification", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		flowAlias := fmt.Sprintf("verify-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      flowAlias,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Verified test flow",
				ProviderId:  "basic-flow",
				Executions: []keycloakv1beta1.AuthenticationExecution{
					{
						Authenticator: "auth-cookie",
						Requirement:   "ALTERNATIVE",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, flow)
		})

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Flow did not become ready")

		kc := getInternalKeycloakClient(t)
		flows, err := kc.GetAuthenticationFlows(ctx, realmName)
		require.NoError(t, err, "Failed to list flows from Keycloak")

		found := false
		for _, f := range flows {
			if f.Alias != nil && *f.Alias == flowAlias {
				found = true
				break
			}
		}
		require.True(t, found, "Flow %s not found in Keycloak", flowAlias)
		t.Logf("Flow %s verified in Keycloak", flowAlias)
	})

	t.Run("FlowCleanup", func(t *testing.T) {
		flowAlias := fmt.Sprintf("cleanup-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      flowAlias,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Cleanup test flow",
				ProviderId:  "basic-flow",
				Executions: []keycloakv1beta1.AuthenticationExecution{
					{
						Authenticator: "auth-cookie",
						Requirement:   "ALTERNATIVE",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		require.NoError(t, k8sClient.Delete(ctx, flow))

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			check := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, check)
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err, "Flow was not deleted from Kubernetes")
		t.Logf("Flow %s cleanup verified", flowAlias)
	})

	t.Run("MissingRealmRef", func(t *testing.T) {
		flowAlias := fmt.Sprintf("no-realm-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      flowAlias,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: "nonexistent-realm"},
				Alias:       flowAlias,
				Description: "Missing realm flow",
				ProviderId:  "basic-flow",
				Executions: []keycloakv1beta1.AuthenticationExecution{
					{
						Authenticator: "auth-cookie",
						Requirement:   "ALTERNATIVE",
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, flow)
		})

		// Should not become ready
		time.Sleep(5 * time.Second)
		updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      flow.Name,
			Namespace: flow.Namespace,
		}, updated))
		require.False(t, updated.Status.Ready, "Flow should not be ready with missing realm")
		t.Logf("Flow %s correctly not ready: %s", flowAlias, updated.Status.Message)
	})
}
