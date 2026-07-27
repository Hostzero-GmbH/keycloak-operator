package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TestDriftSkip verifies that controllers gated by definitionsMatch skip the
// PUT to Keycloak when the desired state already matches what's stored. The
// three sub-tests cover the comparator paths most likely to silently regress:
//
//   - Unordered string-array equality on a real KeycloakClient with reordered
//     redirectUris (set-equality path of valuesMatch).
//   - The IdP clientSecret mask wrapper (idpDefinitionsMatch). This is the
//     scenario the original drift loop hit hardest: every reconcile pushes
//     the secret, Keycloak masks it on GET, so naive byte-compare always
//     fires drift.
//   - The realm smtpServer.password mask wrapper (realmDefinitionsMatch),
//     same shape as the IdP case but on KeycloakRealm.
//
// All three assertions rely on the operator's V(1) "already in sync, skipping
// update" log line as direct evidence the comparator returned true and the
// PUT was bypassed. The chart's dev values run zap with development:true, so
// debug-level controller-runtime logs reach the pod stdout.
func TestDriftSkip(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, _ := getOrCreateInstance(t)

	t.Run("KeycloakClient_UnorderedRedirectUrisSkipsUpdate", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, "drift-client")

		clientName := fmt.Sprintf("drift-client-%d", time.Now().UnixNano())
		clientDef := rawJSON(`{
			"enabled": true,
			"publicClient": true,
			"redirectUris": ["https://a.example.com/*", "https://b.example.com/*", "https://c.example.com/*"],
			"webOrigins": ["https://a.example.com", "https://b.example.com"]
		}`)

		kcClient := &keycloakv1beta1.KeycloakClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakClientSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				ClientId:   strPtr(clientName),
				Definition: &clientDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcClient))
		t.Cleanup(func() { k8sClient.Delete(ctx, kcClient) })

		waitForKCClientReady(t, kcClient.Name, kcClient.Namespace)

		updated := &keycloakv1beta1.KeycloakClient{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: kcClient.Name, Namespace: kcClient.Namespace}, updated))
		require.NotEmpty(t, updated.Status.ClientUUID)

		// Trigger a forced reconcile by patching an annotation. The CR spec
		// hasn't changed and Keycloak still has the same redirectUris the
		// controller pushed at create time, so definitionsMatch must report
		// equality and the controller must skip UpdateClient.
		since := time.Now().UTC()
		bumpReconcile(t, updated)

		assertSkipLogged(t, since, "client already in sync, skipping update", "clientId", clientName)

		// Sanity check: status stayed Ready, ObservedGeneration tracks Generation.
		final := &keycloakv1beta1.KeycloakClient{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: kcClient.Name, Namespace: kcClient.Namespace}, final))
		require.True(t, final.Status.Ready, "client should remain Ready after skipped reconcile")
		require.Equal(t, final.Generation, final.Status.ObservedGeneration, "ObservedGeneration should track Generation")
	})

	t.Run("KeycloakIdentityProvider_SecretMaskNoLoop", func(t *testing.T) {
		realmName := createTestRealm(t, instanceName, "drift-idp")

		idpName := fmt.Sprintf("drift-idp-%d", time.Now().UnixNano())

		// configSecretRef supplies the real clientSecret. Keycloak masks it
		// on GET as "**********", which is the exact case idpDefinitionsMatch
		// strips from both sides.
		idpSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName + "-cfg",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"clientId":     "drift-client-id",
				"clientSecret": "drift-client-secret-value",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idpSecret))
		t.Cleanup(func() { k8sClient.Delete(ctx, idpSecret) })

		idpDef := rawJSON(`{
			"providerId": "oidc",
			"enabled": true,
			"config": {
				"authorizationUrl": "https://idp.example.com/auth",
				"tokenUrl": "https://idp.example.com/token",
				"defaultScope": "openid"
			}
		}`)

		idp := &keycloakv1beta1.KeycloakIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      idpName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
				RealmRef:        &keycloakv1beta1.ResourceRef{Name: realmName},
				Alias:           strPtr(idpName),
				ConfigSecretRef: &keycloakv1beta1.IDPConfigSecretRef{Name: idpSecret.Name},
				Definition:      idpDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, idp))
		t.Cleanup(func() { k8sClient.Delete(ctx, idp) })

		waitForIDPReady(t, idp.Name, idp.Namespace)

		// Confirm Keycloak is masking clientSecret as expected — that's what
		// drives the masking branch in idpDefinitionsMatch. If Keycloak ever
		// changes the mask string, this assertion fails fast and tells us.
		kc := getInternalKeycloakClient(t)
		raw, err := kc.GetIdentityProviderRaw(ctx, realmName, idpName)
		require.NoError(t, err)
		var idpMap map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &idpMap))
		cfg, _ := idpMap["config"].(map[string]interface{})
		require.NotNil(t, cfg, "config map missing on IdP read-back")
		require.Equal(t, "**********", cfg["clientSecret"],
			"Keycloak no longer masks IdP clientSecret as **********; idpDefinitionsMatch needs revisiting")

		// Force a reconcile and assert the controller logs the skip line.
		// Without idpDefinitionsMatch, current would carry "**********" while
		// desired carries the real secret, so naive comparison would fire
		// drift and PUT every reconcile (the original bug).
		updated := &keycloakv1beta1.KeycloakIdentityProvider{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, updated))

		since := time.Now().UTC()
		bumpReconcile(t, updated)

		assertSkipLogged(t, since, "identity provider already in sync, skipping update", "alias", idpName)

		final := &keycloakv1beta1.KeycloakIdentityProvider{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}, final))
		require.True(t, final.Status.Ready, "IdP should remain Ready after skipped reconcile")
	})

	t.Run("KeycloakRealm_SmtpSecretMaskNoLoop", func(t *testing.T) {
		realmName := fmt.Sprintf("drift-smtp-realm-%d", time.Now().UnixNano())

		smtpSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName + "-smtp",
				Namespace: testNamespace,
			},
			StringData: map[string]string{
				"user":     "drift-smtp@example.com",
				"password": "drift-smtp-password",
			},
		}
		require.NoError(t, k8sClient.Create(ctx, smtpSecret))
		t.Cleanup(func() { k8sClient.Delete(ctx, smtpSecret) })

		realm := &keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      realmName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakRealmSpec{
				InstanceRef:   &keycloakv1beta1.ResourceRef{Name: instanceName},
				RealmName:     strPtr(realmName),
				SmtpSecretRef: &keycloakv1beta1.SmtpSecretRefSpec{Name: smtpSecret.Name},
				Definition: rawJSON(`{
					"enabled": true,
					"smtpServer": {
						"host": "smtp.example.com",
						"port": "587",
						"auth": "true"
					}
				}`),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, realm))
		t.Cleanup(func() { k8sClient.Delete(ctx, realm) })

		waitForRealmReady(t, realm.Name, realm.Namespace)

		// Verify Keycloak is masking smtpServer.password (what realmDefinitionsMatch
		// keys off). Same fail-fast intent as the IdP test.
		kc := getInternalKeycloakClient(t)
		raw, err := kc.GetRealmRaw(ctx, realmName)
		require.NoError(t, err)
		var realmMap map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &realmMap))
		smtp, _ := realmMap["smtpServer"].(map[string]interface{})
		require.NotNil(t, smtp, "smtpServer map missing on realm read-back")
		require.Equal(t, "**********", smtp["password"],
			"Keycloak no longer masks smtpServer.password as **********; realmDefinitionsMatch needs revisiting")

		updated := &keycloakv1beta1.KeycloakRealm{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: realm.Name, Namespace: realm.Namespace}, updated))

		since := time.Now().UTC()
		bumpReconcile(t, updated)

		assertSkipLogged(t, since, "realm already in sync, skipping update", "realm", realmName)

		final := &keycloakv1beta1.KeycloakRealm{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: realm.Name, Namespace: realm.Namespace}, final))
		require.True(t, final.Status.Ready, "realm should remain Ready after skipped reconcile")
	})

	t.Run("KeycloakOrganization_DomainsVerifiedNoLoop", func(t *testing.T) {
		// Organizations require Keycloak 26+; skip on older instances.
		instance := &keycloakv1beta1.KeycloakInstance{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, instance))
		if instance.Status.Version == "" || instance.Status.Version[0:2] < "26" {
			t.Skip("Organizations require Keycloak 26.0.0 or later")
		}

		realmName := createTestRealmWithOrganizations(t, instanceName, "drift-org")

		orgName := fmt.Sprintf("drift-org-%d", time.Now().UnixNano())
		// The spec omits domains[].verified. Keycloak sets it on read, which is
		// the exact case organizationDefinitionsMatch strips so a naive
		// byte-compare doesn't fire drift and PUT every reconcile.
		orgDef := rawJSON(fmt.Sprintf(`{
			"alias": "%s",
			"enabled": true,
			"domains": [{"name": "%s.example.com"}]
		}`, orgName, orgName))

		org := &keycloakv1beta1.KeycloakOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakOrganizationSpec{
				Name:       strPtr(orgName),
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: orgDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, org))
		t.Cleanup(func() { k8sClient.Delete(ctx, org) })

		waitForOrganizationReady(t, org.Name, org.Namespace)

		// Confirm Keycloak echoes domains[].verified on read — that's what
		// drives the stripping branch in organizationDefinitionsMatch. If
		// Keycloak stops doing this, fail fast rather than silently.
		ready := &keycloakv1beta1.KeycloakOrganization{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: org.Name, Namespace: org.Namespace}, ready))
		kc := getInternalKeycloakClient(t)
		raw, err := kc.GetOrganizationRaw(ctx, realmName, ready.Status.OrganizationID)
		require.NoError(t, err)
		var orgMap map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &orgMap))
		domains, _ := orgMap["domains"].([]interface{})
		require.NotEmpty(t, domains, "domains missing on organization read-back")
		firstDomain, _ := domains[0].(map[string]interface{})
		require.NotNil(t, firstDomain, "domain entry not an object on read-back")
		require.Contains(t, firstDomain, "verified",
			"Keycloak no longer echoes domains[].verified; organizationDefinitionsMatch may need revisiting")

		// Force a reconcile and assert the controller logs the skip line.
		since := time.Now().UTC()
		bumpReconcile(t, ready)

		assertSkipLogged(t, since, "organization already in sync, skipping update", "name", orgName)

		final := &keycloakv1beta1.KeycloakOrganization{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: org.Name, Namespace: org.Namespace}, final))
		require.True(t, final.Status.Ready, "organization should remain Ready after skipped reconcile")
	})
}

// bumpReconcile patches an annotation onto the given object to force the
// controller's For() watch to fire a reconcile event. Spec generation is
// unchanged because annotations live on metadata, so observed-generation
// tracking is unaffected. Returns once the watch event has had a chance
// to fire (caller is expected to assert behaviour afterwards).
func bumpReconcile(t *testing.T, obj client.Object) {
	t.Helper()
	annos := obj.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos["test/drift-skip-trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
	obj.SetAnnotations(annos)
	require.NoError(t, k8sClient.Update(ctx, obj))
	// Give the controller-runtime workqueue a moment to process the event.
	// 3s comfortably covers a no-op reconcile (Get+definitionsMatch+log+
	// status update with no Keycloak PUT) on the kind cluster.
	time.Sleep(3 * time.Second)
}

// assertSkipLogged tails the operator pod logs since `since` and asserts the
// expected V(1) "already in sync, skipping update" line was emitted with the
// expected structured field (e.g. clientId="foo", alias="bar"). Polls for up
// to 30s because the operator's stdout buffering plus kubectl's --since-time
// rounding can briefly hide a log line that's already been written.
func assertSkipLogged(t *testing.T, since time.Time, msg, kvKey, kvValue string) {
	t.Helper()
	operatorNS := os.Getenv("OPERATOR_NAMESPACE")
	if operatorNS == "" {
		operatorNS = "keycloak-operator"
	}
	// kubectl logs --since-time is RFC3339 with second precision; round down
	// one second so we don't accidentally crop the line we're looking for.
	sinceFlag := since.Add(-1 * time.Second).Format(time.RFC3339)

	deadline := time.Now().Add(30 * time.Second)
	var lastOut string
	for time.Now().Before(deadline) {
		out, err := exec.Command(
			"kubectl", "logs",
			"-n", operatorNS,
			"-l", "app.kubernetes.io/name=keycloak-operator",
			"--since-time="+sinceFlag,
			"--tail=2000",
		).CombinedOutput()
		if err == nil {
			lastOut = string(out)
			if logsContainSkip(lastOut, msg, kvKey, kvValue) {
				t.Logf("Drift-skip log observed: %q with %s=%q", msg, kvKey, kvValue)
				return
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Dump the captured tail to make a failure debuggable without re-running.
	t.Fatalf(
		"expected operator log line %q with %s=%q since %s, but no matching line found.\n--- last operator log tail ---\n%s",
		msg, kvKey, kvValue, sinceFlag, truncateForLog(lastOut, 4000),
	)
}

// logsContainSkip walks the operator log line-by-line looking for one that
// contains both the message and the structured kv pair. zap dev format emits
// the message as plain text and the structured fields as a trailing JSON
// object, e.g.:
//
//	2026-...  DEBUG  ... client already in sync, skipping update {"clientId": "foo", ...}
//
// so a substring check is sufficient and resilient to field reordering.
func logsContainSkip(logs, msg, kvKey, kvValue string) bool {
	kvNeedle := fmt.Sprintf(`"%s": "%s"`, kvKey, kvValue)
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, msg) && strings.Contains(line, kvNeedle) {
			return true
		}
	}
	return false
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...[truncated " + fmt.Sprint(len(s)-n) + " bytes]...\n" + s[len(s)-n:]
}

// waitForKCClientReady waits for a KeycloakClient to reach Ready status.
func waitForKCClientReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakClient{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakClient %s/%s did not become ready", namespace, name)
}

// waitForIDPReady waits for a KeycloakIdentityProvider to reach Ready status.
func waitForIDPReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakIdentityProvider{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakIdentityProvider %s/%s did not become ready", namespace, name)
}

// waitForRealmReady waits for a KeycloakRealm to reach Ready status.
func waitForRealmReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakRealm{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakRealm %s/%s did not become ready", namespace, name)
}

// waitForOrganizationReady waits for a KeycloakOrganization to reach Ready status.
func waitForOrganizationReady(t *testing.T, name, namespace string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		updated := &keycloakv1beta1.KeycloakOrganization{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updated); err != nil {
			return false, nil
		}
		return updated.Status.Ready, nil
	})
	require.NoError(t, err, "KeycloakOrganization %s/%s did not become ready", namespace, name)
}
