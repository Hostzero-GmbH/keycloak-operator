package e2e

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// adminAPIScope is the built-in client scope Keycloak 26.8.2 introduced. From
// that release on, the Admin REST API rejects tokens that do not carry it.
// admin-cli gets it on upgrade; any other client has to be given it explicitly.
const adminAPIScope = "kc-admin-api"

// TestKeycloakInstanceClientCredentialsE2E covers auth.clientCredentials: the
// operator logs in as a confidential client's service account instead of the
// admin user, and manages a realm, a client and a protocol mapper with it.
func TestKeycloakInstanceClientCredentialsE2E(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	kc := getInternalKeycloakClient(t)
	suffix := time.Now().UnixNano()
	clientID := fmt.Sprintf("e2e-operator-cc-%d", suffix)
	clientSecret := fmt.Sprintf("e2e-secret-%d", suffix)

	// The operator's own client is set up directly in the master realm, the
	// way an administrator would prepare it before installing the operator.
	clientDef, err := json.Marshal(map[string]any{
		"clientId":                  clientID,
		"secret":                    clientSecret,
		"publicClient":              false,
		"serviceAccountsEnabled":    true,
		"standardFlowEnabled":       false,
		"directAccessGrantsEnabled": false,
	})
	require.NoError(t, err)
	clientUUID, err := kc.CreateClient(ctx, "master", clientDef)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = kc.DeleteClient(ctx, "master", clientUUID)
	})

	serviceAccount, err := kc.GetClientServiceAccount(ctx, "master", clientUUID)
	require.NoError(t, err)
	require.NotNil(t, serviceAccount.ID)
	adminRole, err := kc.GetRealmRole(ctx, "master", "admin")
	require.NoError(t, err)
	require.NoError(t, kc.AddRealmRolesToUser(ctx, "master", *serviceAccount.ID, []keycloak.RoleRepresentation{*adminRole}))

	// Older Keycloak versions do not have the scope and do not need it.
	scopes, err := kc.GetClientScopes(ctx, "master")
	require.NoError(t, err)
	scopeAssigned := false
	for _, scope := range scopes {
		if scope.Name != nil && *scope.Name == adminAPIScope {
			require.NotNil(t, scope.ID)
			require.NoError(t, kc.AddClientDefaultScope(ctx, "master", clientUUID, *scope.ID))
			scopeAssigned = true
		}
	}
	t.Logf("Operator client %s created (%s scope assigned: %t)", clientID, adminAPIScope, scopeAssigned)

	secretName := fmt.Sprintf("operator-cc-%d", suffix)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
		StringData: map[string]string{
			"client-id":     clientID,
			"client-secret": clientSecret,
		},
	}
	require.NoError(t, k8sClient.Create(ctx, secret))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, secret)
	})

	instanceName := fmt.Sprintf("cc-instance-%d", suffix)
	instance := &keycloakv1beta1.KeycloakInstance{
		ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakInstanceSpec{
			BaseUrl: getKeycloakInternalURL(),
			Auth: keycloakv1beta1.AuthSpec{
				ClientCredentials: &keycloakv1beta1.ClientCredentialsSpec{
					SecretRef: keycloakv1beta1.ClientCredentialsSecretRefSpec{Name: secretName},
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, instance))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, instance)
	})

	updatedInstance := &keycloakv1beta1.KeycloakInstance{}
	waitForReady(t, instanceName, testNamespace, updatedInstance, func() bool {
		return updatedInstance.Status.Ready
	})
	t.Logf("KeycloakInstance %s is ready with client credentials, version: %s", instanceName, updatedInstance.Status.Version)

	// A realm other than master shows the service account's token is accepted
	// across realms, not only for the realm that issued it.
	realmName := createTestRealm(t, instanceName, "cc")

	kcClientName := fmt.Sprintf("cc-client-%d", suffix)
	kcClientDef := rawJSON(`{"enabled": true, "protocol": "openid-connect"}`)
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: kcClientName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakClientSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
			ClientId:   strPtr(kcClientName),
			Definition: &kcClientDef,
		},
	}
	require.NoError(t, k8sClient.Create(ctx, kcClient))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, kcClient)
	})
	updatedClient := &keycloakv1beta1.KeycloakClient{}
	waitForReady(t, kcClientName, testNamespace, updatedClient, func() bool {
		return updatedClient.Status.Ready && updatedClient.Status.ClientUUID != ""
	})

	// Keycloak 26.8.2 also restricted protocol mapper management to callers
	// who may manage the realm, so cover a mapper as well.
	mapperName := fmt.Sprintf("cc-mapper-%d", suffix)
	mapper := &keycloakv1beta1.KeycloakProtocolMapper{
		ObjectMeta: metav1.ObjectMeta{Name: mapperName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
			ClientRef: &keycloakv1beta1.ResourceRef{Name: kcClientName},
			Name:      strPtr(mapperName),
			Definition: rawJSON(`{
				"protocol": "openid-connect",
				"protocolMapper": "oidc-usermodel-attribute-mapper",
				"config": {
					"user.attribute": "department",
					"claim.name": "department",
					"jsonType.label": "String",
					"access.token.claim": "true"
				}
			}`),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, mapper))
	t.Cleanup(func() {
		k8sClient.Delete(ctx, mapper)
	})
	updatedMapper := &keycloakv1beta1.KeycloakProtocolMapper{}
	waitForReady(t, mapperName, testNamespace, updatedMapper, func() bool {
		return updatedMapper.Status.Ready
	})

	// The mapper must exist in Keycloak, not only report Ready.
	mappers, err := kc.GetClientProtocolMappers(ctx, realmName, updatedClient.Status.ClientUUID)
	require.NoError(t, err)
	found := false
	for _, m := range mappers {
		if m.Name != nil && *m.Name == mapperName {
			found = true
		}
	}
	require.True(t, found, "protocol mapper %s not found in Keycloak", mapperName)
}
