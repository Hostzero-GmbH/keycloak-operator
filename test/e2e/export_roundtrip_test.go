package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/export"
)

func TestExportRoundTrip(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	instance := &keycloakv1beta1.KeycloakInstance{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: instanceNS}, instance))
	if instance.Status.Version == "" || instance.Status.Version[0:2] < "26" {
		t.Skip("Organizations require Keycloak 26.0.0 or later")
	}

	suffix := time.Now().UnixNano()
	sourceRealm := createTestRealmWithOrganizations(t, instanceName, "rt-src")

	clientID := fmt.Sprintf("rt-client-%d", suffix)
	scopeName := fmt.Sprintf("rt-scope-%d", suffix)
	groupName := fmt.Sprintf("rt-group-%d", suffix)
	userName := fmt.Sprintf("rt-user-%d", suffix)
	roleName := fmt.Sprintf("rt-role-%d", suffix)
	mapperName := fmt.Sprintf("rt-mapper-%d", suffix)
	idpAlias := fmt.Sprintf("rt-idp-%d", suffix)
	idpMapperName := fmt.Sprintf("rt-idp-mapper-%d", suffix)
	orgName := fmt.Sprintf("rt-org-%d", suffix)
	componentName := fmt.Sprintf("rt-component-%d", suffix)

	clientDef := rawJSON(`{"enabled": true, "protocol": "openid-connect", "publicClient": true}`)
	kcClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + clientID, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakClientSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			ClientId:   strPtr(clientID),
			Definition: &clientDef,
		},
	}
	createAndWaitReady(t, kcClient)

	scopeDef := rawJSON(`{"protocol": "openid-connect", "description": "round-trip scope"}`)
	scope := &keycloakv1beta1.KeycloakClientScope{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + scopeName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakClientScopeSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(scopeName),
			Definition: scopeDef,
		},
	}
	createAndWaitReady(t, scope)

	groupDef := rawJSON(`{}`)
	group := &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + groupName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakGroupSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(groupName),
			Definition: groupDef,
		},
	}
	createAndWaitReady(t, group)

	userDef := rawJSON(`{"enabled": true}`)
	user := &keycloakv1beta1.KeycloakUser{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + userName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakUserSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Username:   strPtr(userName),
			Definition: &userDef,
		},
	}
	createAndWaitReady(t, user)

	roleDef := rawJSON(`{"description": "round-trip role"}`)
	role := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + roleName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRoleSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(roleName),
			Definition: roleDef,
		},
	}
	createAndWaitReady(t, role)

	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + userName + "-" + roleName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
			Subject: keycloakv1beta1.RoleMappingSubject{
				UserRef: &keycloakv1beta1.ResourceRef{Name: user.Name},
			},
			RoleRef: &keycloakv1beta1.ResourceRef{Name: role.Name},
		},
	}
	createAndWaitReady(t, mapping)

	pmDef := rawJSON(`{
		"protocol": "openid-connect",
		"protocolMapper": "oidc-usermodel-attribute-mapper",
		"config": {
			"user.attribute": "department",
			"claim.name": "department",
			"jsonType.label": "String",
			"id.token.claim": "true",
			"access.token.claim": "true"
		}
	}`)
	pm := &keycloakv1beta1.KeycloakProtocolMapper{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + mapperName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
			ClientRef:  &keycloakv1beta1.ResourceRef{Name: kcClient.Name},
			Name:       strPtr(mapperName),
			Definition: pmDef,
		},
	}
	createAndWaitReady(t, pm)

	orgDef := rawJSON(fmt.Sprintf(`{
		"alias": "%s",
		"enabled": true,
		"domains": [{"name": "%s.example.com", "verified": false}]
	}`, orgName, orgName))
	org := &keycloakv1beta1.KeycloakOrganization{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + orgName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakOrganizationSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(orgName),
			Definition: orgDef,
		},
	}
	createAndWaitReady(t, org)

	idpDef := rawJSON(`{
		"providerId": "oidc",
		"enabled": true,
		"config": {
			"clientId": "rt-idp",
			"clientSecret": "rt-idp-secret",
			"authorizationUrl": "https://idp.example.com/auth",
			"tokenUrl": "https://idp.example.com/token"
		}
	}`)
	idp := &keycloakv1beta1.KeycloakIdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + idpAlias, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
			RealmRef:        &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			OrganizationRef: &keycloakv1beta1.ResourceRef{Name: org.Name},
			Alias:           strPtr(idpAlias),
			Definition:      idpDef,
		},
	}
	createAndWaitReady(t, idp)

	idpMapperDef := rawJSON(`{
		"identityProviderMapper": "hardcoded-attribute-idp-mapper",
		"config": {
			"syncMode": "INHERIT",
			"attribute": "source",
			"attribute.value": "roundtrip"
		}
	}`)
	idpMapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + idpMapperName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakIdentityProviderMapperSpec{
			IdentityProviderRef: keycloakv1beta1.ResourceRef{Name: idp.Name},
			Name:                strPtr(idpMapperName),
			Definition:          idpMapperDef,
		},
	}
	createAndWaitReady(t, idpMapper)

	componentDef := rawJSON(`{
		"providerId": "trusted-hosts",
		"providerType": "org.keycloak.services.clientregistration.policy.ClientRegistrationPolicy",
		"config": {
			"host-sending-registration-request-must-match": ["true"],
			"client-uris-must-match": ["true"]
		}
	}`)
	component := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + componentName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(componentName),
			Definition: componentDef,
		},
	}
	createAndWaitReady(t, component)

	targetRealmCR := fmt.Sprintf("rt-dst-%d", suffix)
	targetRealm := createNamedRealmWithOrganizations(t, instanceName, targetRealmCR)

	kc := getInternalKeycloakClient(t)
	exporter := export.NewExporter(kc, ctrl.Log.WithName("export-roundtrip"), export.ExporterOptions{
		Realm:           sourceRealm,
		TargetNamespace: testNamespace,
		InstanceRef:     instanceName,
		RealmRef:        targetRealmCR,
		SkipDefaults:    true,
	})
	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	expected := map[string]string{
		"KeycloakClient":                 clientID,
		"KeycloakClientScope":            scopeName,
		"KeycloakGroup":                  groupName,
		"KeycloakUser":                   userName,
		"KeycloakRole":                   roleName,
		"KeycloakRoleMapping":            userName + "-" + roleName,
		"KeycloakProtocolMapper":         clientID + "-" + mapperName,
		"KeycloakOrganization":           orgName,
		"KeycloakIdentityProvider":       idpAlias,
		"KeycloakIdentityProviderMapper": idpAlias + "-" + idpMapperName,
		"KeycloakComponent":              "clientregistrationpolicy-" + componentName,
	}

	var applied []client.Object
	found := map[string]bool{}
	for _, res := range resources {
		if res.Kind == "KeycloakRealm" {
			continue
		}
		want, ok := expected[res.Kind]
		if !ok || res.Name != want {
			continue
		}
		obj, ok := res.Object.(client.Object)
		require.True(t, ok, "exported %s %s is not a client.Object", res.Kind, res.Name)
		require.NoError(t, k8sClient.Create(ctx, obj), "create exported %s/%s", res.Kind, res.Name)
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, obj) })
		applied = append(applied, obj)
		found[res.Kind] = true
	}
	for kind := range expected {
		require.True(t, found[kind], "export did not include %s %q", kind, expected[kind])
	}

	roundTripTimeout := 2 * time.Minute
	for _, obj := range applied {
		waitObjectReady(t, obj, roundTripTimeout)
	}

	targetOrg := &keycloakv1beta1.KeycloakOrganization{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: orgName, Namespace: testNamespace}, targetOrg))
	require.True(t, targetOrg.Status.Ready)
	require.NotEmpty(t, targetOrg.Status.OrganizationID)

	targetIDP := &keycloakv1beta1.KeycloakIdentityProvider{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: idpAlias, Namespace: testNamespace}, targetIDP))
	require.True(t, targetIDP.Status.Ready)
	require.NotNil(t, targetIDP.Spec.OrganizationRef)
	require.Equal(t, orgName, targetIDP.Spec.OrganizationRef.Name)
	require.Equal(t, targetOrg.Status.OrganizationID, targetIDP.Status.OrganizationID)

	raw, err := kc.GetIdentityProviderRaw(ctx, targetRealm, idpAlias)
	require.NoError(t, err)
	var parsed struct {
		OrganizationID string `json:"organizationId"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Equal(t, targetOrg.Status.OrganizationID, parsed.OrganizationID)
}

func createNamedRealmWithOrganizations(t *testing.T, instanceName, realmName string) string {
	t.Helper()
	realm := &keycloakv1beta1.KeycloakRealm{
		ObjectMeta: metav1.ObjectMeta{Name: realmName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRealmSpec{
			InstanceRef: &keycloakv1beta1.ResourceRef{Name: instanceName},
			RealmName:   strPtr(realmName),
			Definition:  rawJSON(`{"enabled": true, "organizationsEnabled": true}`),
		},
	}
	require.NoError(t, k8sClient.Create(ctx, realm))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, realm) })
	waitObjectReady(t, realm, timeout)
	return realmName
}

func createAndWaitReady(t *testing.T, obj client.Object) {
	t.Helper()
	require.NoError(t, k8sClient.Create(ctx, obj))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, obj) })
	waitObjectReady(t, obj, timeout)
}

func waitObjectReady(t *testing.T, obj client.Object, waitTimeout time.Duration) {
	t.Helper()
	key := client.ObjectKeyFromObject(obj)
	err := wait.PollUntilContextTimeout(ctx, interval, waitTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			return false, nil
		}
		return objectReady(obj), nil
	})
	if err != nil {
		_ = k8sClient.Get(ctx, key, obj)
		t.Fatalf("resource did not become ready: %s/%s status=%v", key.Namespace, key.Name, statusMessage(obj))
	}
}

func objectReady(obj client.Object) bool {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return false
	}
	ready := v.Elem().FieldByName("Status").FieldByName("Ready")
	return ready.IsValid() && ready.Kind() == reflect.Bool && ready.Bool()
}

func statusMessage(obj client.Object) string {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return ""
	}
	status := v.Elem().FieldByName("Status")
	if !status.IsValid() {
		return ""
	}
	msg := status.FieldByName("Message")
	st := status.FieldByName("Status")
	parts := ""
	if st.IsValid() && st.Kind() == reflect.String {
		parts = st.String()
	}
	if msg.IsValid() && msg.Kind() == reflect.String && msg.String() != "" {
		if parts != "" {
			parts += ": "
		}
		parts += msg.String()
	}
	return parts
}
