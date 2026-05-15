package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// rawExecutions wraps a JSON literal in a runtime.RawExtension.
func rawExecutions(s string) runtime.RawExtension {
	return runtime.RawExtension{Raw: []byte(s)}
}

func waitForFlowReady(t *testing.T, name string) *keycloakv1beta1.KeycloakAuthenticationFlow {
	t.Helper()
	updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: testNamespace,
		}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "Authentication flow %s did not become ready: %s", name, updated.Status.Message)
	return updated
}

func TestKeycloakAuthenticationFlowE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "authflow")

	t.Run("SimpleFlow", func(t *testing.T) {
		flowAlias := fmt.Sprintf("simple-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Simple test flow",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(`[
					{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"},
					{"authenticator":"auth-spnego","requirement":"DISABLED"}
				]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		updated := waitForFlowReady(t, flow.Name)
		require.NotEmpty(t, updated.Status.FlowID, "Flow ID should be set")
		require.NotEmpty(t, updated.Status.ResourcePath, "Resource path should be set")
		t.Logf("Flow %s is ready with ID %s", flowAlias, updated.Status.FlowID)
	})

	t.Run("FlowWithSubFlows", func(t *testing.T) {
		flowAlias := fmt.Sprintf("nested-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Nested test flow",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(fmt.Sprintf(`[
					{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"},
					{
						"subFlow":{
							"alias":"%s-forms",
							"description":"Form sub-flow",
							"providerId":"basic-flow",
							"executions":[
								{"authenticator":"auth-username-password-form","requirement":"REQUIRED"}
							]
						},
						"requirement":"ALTERNATIVE"
					}
				]`, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)
		t.Logf("Nested flow %s is ready", flowAlias)
	})

	t.Run("FlowWithSiblingExecutionsAndFormFlow", func(t *testing.T) {
		flowAlias := fmt.Sprintf("registration-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Registration flow using sibling executions shape",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(fmt.Sprintf(`[
					{
						"subFlow":{
							"alias":"%s-registration-form",
							"providerId":"form-flow",
							"description":"Registration form sub-flow"
						},
						"requirement":"REQUIRED",
						"executions":[
							{"authenticator":"registration-user-creation","requirement":"REQUIRED"},
							{"authenticator":"registration-password-action","requirement":"REQUIRED"},
							{"authenticator":"registration-terms-and-conditions","requirement":"DISABLED"}
						]
					}
				]`, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)
		t.Logf("Registration flow %s is ready", flowAlias)
	})

	t.Run("FlowWithDeeplyNestedSubFlows", func(t *testing.T) {
		// Mirrors Keycloak's built-in browser flow: a basic-flow "forms"
		// sub-flow that contains a CONDITIONAL basic-flow "conditional 2FA"
		// sub-flow which hosts the OTP authenticators.
		flowAlias := fmt.Sprintf("browser-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Browser flow with a nested conditional sub-flow",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(fmt.Sprintf(`[
					{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"},
					{
						"subFlow":{
							"alias":"%s-forms",
							"providerId":"basic-flow",
							"executions":[
								{"authenticator":"auth-username-password-form","requirement":"REQUIRED"},
								{
									"subFlow":{
										"alias":"%s-conditional-2fa",
										"providerId":"basic-flow",
										"executions":[
											{"authenticator":"conditional-user-configured","requirement":"REQUIRED"},
											{"authenticator":"auth-otp-form","requirement":"REQUIRED"}
										]
									},
									"requirement":"CONDITIONAL"
								}
							]
						},
						"requirement":"ALTERNATIVE"
					}
				]`, flowAlias, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)
		t.Logf("Deeply nested flow %s is ready", flowAlias)
	})

	t.Run("RealmWithDeferredCustomBrowserFlow", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		customRealmName := fmt.Sprintf("test-realm-authflow-binding-%d", time.Now().UnixNano())
		flowAlias := fmt.Sprintf("custom-browser-%d", time.Now().UnixNano())
		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: customRealmName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"enabled": true,
					"browserFlow": "%s"
				}`, customRealmName, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Realm with deferred custom browserFlow did not become ready")

		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: customRealmName},
				Alias:       flowAlias,
				Description: "Custom browser flow bound by KeycloakRealm",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)

		kc := getInternalKeycloakClient(t)
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			realmRaw, err := kc.GetRealmRaw(ctx, customRealmName)
			if err != nil {
				return false, nil
			}
			var realmData map[string]interface{}
			if err := json.Unmarshal(realmRaw, &realmData); err != nil {
				return false, err
			}
			return realmData["browserFlow"] == flowAlias, nil
		})
		require.NoError(t, err, "Realm did not bind browserFlow to the custom flow")
		t.Logf("Realm %s bound browserFlow to %s", customRealmName, flowAlias)
	})

	// Regression: a realm referencing a not-yet-created
	// firstBrokerLoginFlow must defer the binding instead of
	// crashing Keycloak during realm import.
	t.Run("RealmWithDeferredCustomFirstBrokerLoginFlow", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		customRealmName := fmt.Sprintf("test-realm-fbl-binding-%d", time.Now().UnixNano())
		flowAlias := fmt.Sprintf("custom-fbl-%d", time.Now().UnixNano())
		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: customRealmName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"enabled": true,
					"firstBrokerLoginFlow": "%s"
				}`, customRealmName, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      realm.Name,
				Namespace: realm.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Realm with deferred custom firstBrokerLoginFlow did not become ready")

		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: customRealmName},
				Alias:       flowAlias,
				Description: "Custom first-broker-login flow bound by KeycloakRealm",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"idp-create-user-if-unique","requirement":"ALTERNATIVE"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)

		kc := getInternalKeycloakClient(t)
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			realmRaw, err := kc.GetRealmRaw(ctx, customRealmName)
			if err != nil {
				return false, nil
			}
			var realmData map[string]interface{}
			if err := json.Unmarshal(realmRaw, &realmData); err != nil {
				return false, err
			}
			return realmData["firstBrokerLoginFlow"] == flowAlias, nil
		})
		require.NoError(t, err, "Realm did not bind firstBrokerLoginFlow to the custom flow")
		t.Logf("Realm %s bound firstBrokerLoginFlow to %s", customRealmName, flowAlias)
	})

	t.Run("FlowWithKeycloakVerification", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		flowAlias := fmt.Sprintf("verify-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Verified test flow",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)

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

	t.Run("FlowInPlaceUpdate", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		flowAlias := fmt.Sprintf("inplace-flow-%d", time.Now().UnixNano())
		subFlowAlias := flowAlias + "-forms"
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "initial description",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(fmt.Sprintf(`[
					{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"},
					{"authenticator":"auth-spnego","requirement":"DISABLED"},
					{
						"subFlow":{
							"alias":"%s",
							"providerId":"basic-flow",
							"executions":[
								{"authenticator":"auth-username-password-form","requirement":"REQUIRED"}
							]
						},
						"requirement":"ALTERNATIVE"
					}
				]`, subFlowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		initial := waitForFlowReady(t, flow.Name)
		originalFlowID := initial.Status.FlowID
		require.NotEmpty(t, originalFlowID)

		// Mutate the spec: change a leaf's requirement, drop one leaf,
		// add a new sub-flow child, and tweak the description. Identity
		// of every kept entry stays the same so reconcileChildren must
		// patch in place rather than recreate.
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, flow))
		flow.Spec.Description = "updated description"
		flow.Spec.Executions = rawExecutions(fmt.Sprintf(`[
			{"authenticator":"auth-cookie","requirement":"REQUIRED"},
			{
				"subFlow":{
					"alias":"%s",
					"providerId":"basic-flow",
					"executions":[
						{"authenticator":"auth-username-password-form","requirement":"REQUIRED"},
						{"authenticator":"auth-otp-form","requirement":"REQUIRED"}
					]
				},
				"requirement":"ALTERNATIVE"
			}
		]`, subFlowAlias))
		require.NoError(t, k8sClient.Update(ctx, flow))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ObservedGeneration == updated.Generation, nil
		})
		require.NoError(t, err, "Flow did not converge after in-place update")

		converged := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, converged))
		require.Equal(t, originalFlowID, converged.Status.FlowID, "Flow ID must stay stable across in-place update")

		kc := getInternalKeycloakClient(t)
		execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		require.NoError(t, err)

		var sawAuthCookie, sawSpnego, sawForms bool
		for i := range execs {
			e := execs[i]
			if e.Level == nil || *e.Level != 0 {
				continue
			}
			switch {
			case e.ProviderID != nil && *e.ProviderID == "auth-cookie":
				sawAuthCookie = true
				require.NotNil(t, e.Requirement)
				require.Equal(t, "REQUIRED", *e.Requirement, "auth-cookie requirement should be updated to REQUIRED")
			case e.ProviderID != nil && *e.ProviderID == "auth-spnego":
				sawSpnego = true
			case e.AuthenticationFlow != nil && *e.AuthenticationFlow && e.DisplayName != nil && *e.DisplayName == subFlowAlias:
				sawForms = true
			}
		}
		require.True(t, sawAuthCookie, "auth-cookie leaf must still be present")
		require.False(t, sawSpnego, "auth-spnego leaf should have been removed")
		require.True(t, sawForms, "forms sub-flow must still be present")

		subExecs, err := kc.GetFlowExecutions(ctx, realmName, subFlowAlias)
		require.NoError(t, err)
		var sawUserPass, sawOtp bool
		for _, e := range subExecs {
			if e.Level != nil && *e.Level == 0 && e.ProviderID != nil {
				if *e.ProviderID == "auth-username-password-form" {
					sawUserPass = true
				}
				if *e.ProviderID == "auth-otp-form" {
					sawOtp = true
				}
			}
		}
		require.True(t, sawUserPass, "username/password form must still be present in sub-flow")
		require.True(t, sawOtp, "auth-otp-form must have been added to sub-flow")
		t.Logf("Flow %s updated in place; ID %s preserved", flowAlias, originalFlowID)
	})

	t.Run("FlowBoundToRealmCanBeUpdated", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)

		// Regression coverage for the user-reported case where a flow
		// is referenced from somewhere else (here: a realm's browserFlow
		// override). A delete-and-recreate update path would fail with
		// Keycloak's "Cannot remove authentication flow, it is
		// currently in use"; in-place reconciliation must succeed
		// without releasing the binding.
		customRealmName := fmt.Sprintf("test-realm-inplace-%d", time.Now().UnixNano())
		flowAlias := fmt.Sprintf("realm-bound-flow-%d", time.Now().UnixNano())

		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: customRealmName},
				Alias:       flowAlias,
				Description: "Bound to realm browserFlow",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}]`),
			},
		}

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: customRealmName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName, Namespace: &instanceNS},
				Definition: rawJSON(fmt.Sprintf(`{
					"realm": "%s",
					"enabled": true,
					"browserFlow": "%s"
				}`, customRealmName, flowAlias)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakRealm{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: realm.Name, Namespace: realm.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "Realm with deferred custom browserFlow did not become ready")

		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		initial := waitForFlowReady(t, flow.Name)
		originalFlowID := initial.Status.FlowID

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			kc := getInternalKeycloakClient(t)
			realmRaw, err := kc.GetRealmRaw(ctx, customRealmName)
			if err != nil {
				return false, nil
			}
			var realmData map[string]interface{}
			if err := json.Unmarshal(realmRaw, &realmData); err != nil {
				return false, err
			}
			return realmData["browserFlow"] == flowAlias, nil
		})
		require.NoError(t, err, "Realm did not bind browserFlow to the custom flow")

		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, flow))
		flow.Spec.Executions = rawExecutions(`[
			{"authenticator":"auth-cookie","requirement":"REQUIRED"},
			{"authenticator":"identity-provider-redirector","requirement":"ALTERNATIVE"}
		]`)
		require.NoError(t, k8sClient.Update(ctx, flow))

		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready && updated.Status.ObservedGeneration == updated.Generation, nil
		})
		require.NoError(t, err, "Realm-bound flow did not converge after update")

		converged := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, converged))
		require.Equal(t, originalFlowID, converged.Status.FlowID, "Flow ID must stay stable so realm binding remains valid")
		require.Equal(t, "Ready", converged.Status.Status)
		t.Logf("Realm-bound flow %s updated in place without releasing the binding", flowAlias)
	})

	// Regression for keycloak/keycloak#35765 (new executions all default to
	// priority 0 on KC 25/26, so order is non-deterministic without an
	// explicit priority).
	t.Run("FlowExecutionOrderMatchesSpec", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)
		flowAlias := fmt.Sprintf("ordered-flow-%d", time.Now().UnixNano())
		subFlowA := flowAlias + "-sub-a"
		subFlowB := flowAlias + "-sub-b"
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Order regression for keycloak/keycloak#35765",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(fmt.Sprintf(`[
					{"authenticator":"direct-grant-validate-username","requirement":"REQUIRED"},
					{
						"subFlow":{
							"alias":"%s",
							"providerId":"basic-flow",
							"executions":[
								{"authenticator":"conditional-user-configured","requirement":"REQUIRED"}
							]
						},
						"requirement":"CONDITIONAL"
					},
					{"authenticator":"direct-grant-validate-password","requirement":"REQUIRED"},
					{
						"subFlow":{
							"alias":"%s",
							"providerId":"basic-flow",
							"executions":[
								{"authenticator":"conditional-user-configured","requirement":"REQUIRED"},
								{"authenticator":"direct-grant-validate-otp","requirement":"REQUIRED"}
							]
						},
						"requirement":"CONDITIONAL"
					},
					{"authenticator":"user-session-limits","requirement":"REQUIRED"}
				]`, subFlowA, subFlowB)),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)

		want := []string{
			"direct-grant-validate-username",
			subFlowA,
			"direct-grant-validate-password",
			subFlowB,
			"user-session-limits",
		}

		kc := getInternalKeycloakClient(t)
		execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		require.NoError(t, err)

		var got []string
		for i := range execs {
			e := execs[i]
			if e.Level == nil || *e.Level != 0 {
				continue
			}
			switch {
			case e.AuthenticationFlow != nil && *e.AuthenticationFlow && e.DisplayName != nil:
				got = append(got, *e.DisplayName)
			case e.ProviderID != nil:
				got = append(got, *e.ProviderID)
			}
		}
		require.Equal(t, want, got, "top-level executions must appear in the order declared in the spec")

		subBExecs, err := kc.GetFlowExecutions(ctx, realmName, subFlowB)
		require.NoError(t, err)
		var subBGot []string
		for _, e := range subBExecs {
			if e.Level != nil && *e.Level == 0 && e.ProviderID != nil {
				subBGot = append(subBGot, *e.ProviderID)
			}
		}
		require.Equal(t, []string{"conditional-user-configured", "direct-grant-validate-otp"}, subBGot,
			"sub-flow executions must also appear in the order declared in the spec")
		t.Logf("Flow %s executions are in spec order", flowAlias)
	})

	t.Run("FlowExecutionOrderDriftIsRepaired", func(t *testing.T) {
		skipIfNoKeycloakAccess(t)
		flowAlias := fmt.Sprintf("drift-order-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Order drift regression",
				ProviderId:  "basic-flow",
				Executions: rawExecutions(`[
					{"authenticator":"direct-grant-validate-username","requirement":"REQUIRED"},
					{"authenticator":"direct-grant-validate-password","requirement":"REQUIRED"},
					{"authenticator":"direct-grant-validate-otp","requirement":"REQUIRED"}
				]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		waitForFlowReady(t, flow.Name)

		kc := getInternalKeycloakClient(t)

		// Drift Keycloak directly without touching the CR.
		live, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		require.NoError(t, err)
		var otpID string
		for _, e := range live {
			if e.Level != nil && *e.Level == 0 && e.ProviderID != nil && *e.ProviderID == "direct-grant-validate-otp" {
				require.NotNil(t, e.ID)
				otpID = *e.ID
				break
			}
		}
		require.NotEmpty(t, otpID, "OTP execution must exist in live flow")
		require.NoError(t, kc.RaiseExecutionPriority(ctx, realmName, otpID))
		require.NoError(t, kc.RaiseExecutionPriority(ctx, realmName, otpID))

		drifted, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		require.NoError(t, err)
		var driftedOrder []string
		for _, e := range drifted {
			if e.Level != nil && *e.Level == 0 && e.ProviderID != nil {
				driftedOrder = append(driftedOrder, *e.ProviderID)
			}
		}
		require.Equal(t, []string{
			"direct-grant-validate-otp",
			"direct-grant-validate-username",
			"direct-grant-validate-password",
		}, driftedOrder, "raise-priority must have actually moved the OTP to the top")

		// Force a reconcile without bumping the spec.
		fresh := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: flow.Name, Namespace: flow.Namespace}, fresh))
		bumpReconcile(t, fresh)

		require.Eventually(t, func() bool {
			repaired, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
			if err != nil {
				return false
			}
			var got []string
			for _, e := range repaired {
				if e.Level != nil && *e.Level == 0 && e.ProviderID != nil {
					got = append(got, *e.ProviderID)
				}
			}
			want := []string{
				"direct-grant-validate-username",
				"direct-grant-validate-password",
				"direct-grant-validate-otp",
			}
			return len(got) == len(want) && got[0] == want[0] && got[1] == want[1] && got[2] == want[2]
		}, 30*time.Second, 500*time.Millisecond, "controller did not repair execution order drift")
		t.Logf("Flow %s execution order drift was detected and repaired", flowAlias)
	})

	t.Run("FlowCleanup", func(t *testing.T) {
		flowAlias := fmt.Sprintf("cleanup-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Cleanup test flow",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))

		waitForFlowReady(t, flow.Name)

		require.NoError(t, k8sClient.Delete(ctx, flow))

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
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
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: "nonexistent-realm"},
				Alias:       flowAlias,
				Description: "Missing realm flow",
				ProviderId:  "basic-flow",
				Executions:  rawExecutions(`[{"authenticator":"auth-cookie","requirement":"ALTERNATIVE"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

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

	t.Run("InvalidExecutionsShape", func(t *testing.T) {
		flowAlias := fmt.Sprintf("invalid-flow-%d", time.Now().UnixNano())
		flow := &keycloakv1beta1.KeycloakAuthenticationFlow{
			ObjectMeta: metav1.ObjectMeta{Name: flowAlias, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakAuthenticationFlowSpec{
				RealmRef:    &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:       flowAlias,
				Description: "Invalid executions shape",
				ProviderId:  "basic-flow",
				// requirement field omitted on purpose - the controller
				// must surface a validation error.
				Executions: rawExecutions(`[{"authenticator":"auth-cookie"}]`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, flow))
		t.Cleanup(func() { k8sClient.Delete(ctx, flow) })

		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      flow.Name,
				Namespace: flow.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Status == "InvalidSpec", nil
		})
		require.NoError(t, err, "Controller did not report InvalidSpec for malformed executions")

		updated := &keycloakv1beta1.KeycloakAuthenticationFlow{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      flow.Name,
			Namespace: flow.Namespace,
		}, updated))
		require.False(t, updated.Status.Ready)
		require.Contains(t, updated.Status.Message, "requirement is required")
		t.Logf("Flow %s correctly rejected: %s", flowAlias, updated.Status.Message)
	})
}
