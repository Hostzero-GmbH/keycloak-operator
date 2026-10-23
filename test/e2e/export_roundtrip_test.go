package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
	kc := getInternalKeycloakClient(t)

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

	// Second tier of the fixture: the shapes behind the repo's export bug
	// history. Short suffixes keep the exporter's derived CR names under the
	// 63-character limit, which sanitizeName would otherwise truncate.
	short := suffix % 1000000
	childGroupName := fmt.Sprintf("rt-child-%d", short)
	shortRoleName := fmt.Sprintf("rt-r2-%d", short)
	compositeName := fmt.Sprintf("rt-composite-%d", short)
	confClientID := fmt.Sprintf("rt-conf-%d", short)
	clientRoleName := fmt.Sprintf("rt-crole-%d", short)
	scopeMapperName := fmt.Sprintf("rt-scope-mapper-%d", short)
	samlClientID := fmt.Sprintf("rt-saml-%d", short)
	ldapName := fmt.Sprintf("rt-ldap-%d", short)
	ldapMapperName := "dept"
	ldapSecretName := fmt.Sprintf("rt-ldap-bind-%d", short)
	ldapProviderCR := "userstorageprovider-" + ldapName
	ldapMapperCR := ldapProviderCR + "-ldapstoragemapper-" + ldapMapperName

	// Nested group (#53) with a role mapping on the child (#134).
	childGroup := &keycloakv1beta1.KeycloakGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + childGroupName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakGroupSpec{
			ParentGroupRef: &keycloakv1beta1.ResourceRef{Name: group.Name},
			Name:           strPtr(childGroupName),
			Definition:     rawJSON(`{"attributes": {"tier": ["gold"]}}`),
		},
	}
	createAndWaitReady(t, childGroup)

	shortRole := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + shortRoleName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRoleSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:       strPtr(shortRoleName),
			Definition: rawJSON(`{"description": "mapped to groups"}`),
		},
	}
	createAndWaitReady(t, shortRole)

	for _, g := range []*keycloakv1beta1.KeycloakGroup{group, childGroup} {
		createAndWaitReady(t, &keycloakv1beta1.KeycloakRoleMapping{
			ObjectMeta: metav1.ObjectMeta{Name: g.Name + "-" + shortRoleName, Namespace: testNamespace},
			Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
				Subject: keycloakv1beta1.RoleMappingSubject{GroupRef: &keycloakv1beta1.ResourceRef{Name: g.Name}},
				RoleRef: &keycloakv1beta1.ResourceRef{Name: shortRole.Name},
			},
		})
	}

	// Confidential client with a custom optional scope assignment (#51, #126),
	// authorization services (the array-of-objects path in the drift matcher)
	// and a client role (#124).
	confDef := rawJSON(fmt.Sprintf(`{
		"enabled": true,
		"protocol": "openid-connect",
		"publicClient": false,
		"serviceAccountsEnabled": true,
		"authorizationServicesEnabled": true,
		"optionalClientScopes": ["%s", "offline_access"],
		"authorizationSettings": {
			"allowRemoteResourceManagement": false,
			"policyEnforcementMode": "ENFORCING",
			"resources": [{"name": "rt-resource", "uris": ["/api/*"]}]
		}
	}`, scopeName))
	confClient := &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + confClientID, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakClientSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			ClientId:   strPtr(confClientID),
			Definition: &confDef,
		},
	}
	createAndWaitReady(t, confClient)

	clientRole := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + clientRoleName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRoleSpec{
			ClientRef:  &keycloakv1beta1.ResourceRef{Name: confClient.Name},
			Name:       strPtr(clientRoleName),
			Definition: rawJSON(`{"description": "client role"}`),
		},
	}
	createAndWaitReady(t, clientRole)

	// Composite role (#73): members live behind a separate endpoint.
	composite := &keycloakv1beta1.KeycloakRole{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + compositeName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakRoleSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:     strPtr(compositeName),
			Definition: rawJSON(fmt.Sprintf(`{
				"composite": true,
				"composites": {"realm": ["%s"], "client": {"%s": ["%s"]}}
			}`, roleName, confClientID, clientRoleName)),
		},
	}
	createAndWaitReady(t, composite)

	// Protocol mapper on a client scope (#123).
	createAndWaitReady(t, &keycloakv1beta1.KeycloakProtocolMapper{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + scopeMapperName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
			ClientScopeRef: &keycloakv1beta1.ResourceRef{Name: scope.Name},
			Name:           strPtr(scopeMapperName),
			Definition: rawJSON(`{
				"protocol": "openid-connect",
				"protocolMapper": "oidc-hardcoded-claim-mapper",
				"config": {"claim.name": "tenant", "claim.value": "roundtrip", "jsonType.label": "String",
					"id.token.claim": "true", "access.token.claim": "true"}
			}`),
		},
	})

	// SAML client (#131).
	samlDef := rawJSON(`{
		"enabled": true,
		"protocol": "saml",
		"redirectUris": ["https://sp.example.com/saml/*"],
		"attributes": {"saml.assertion.signature": "true", "saml_name_id_format": "email"}
	}`)
	createAndWaitReady(t, &keycloakv1beta1.KeycloakClient{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + samlClientID, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakClientSpec{
			RealmRef:   &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			ClientId:   strPtr(samlClientID),
			Definition: &samlDef,
		},
	})

	// LDAP provider with a masked bind credential (#136, #140, #143) and a
	// mapper under it (#158, #167).
	ldapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ldapSecretName, Namespace: testNamespace},
		StringData: map[string]string{"bindCredential": "rt-bind-password"},
	}
	require.NoError(t, k8sClient.Create(ctx, ldapSecret))
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, ldapSecret) })

	ldap := &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + ldapName, Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			RealmRef:        &keycloakv1beta1.ResourceRef{Name: sourceRealm},
			Name:            strPtr(ldapName),
			ConfigSecretRef: &keycloakv1beta1.ConfigSecretRef{Name: ldapSecretName},
			Definition: rawJSON(`{
				"providerId": "ldap",
				"providerType": "org.keycloak.storage.UserStorageProvider",
				"config": {
					"enabled": ["false"],
					"vendor": ["other"],
					"connectionUrl": ["ldap://ldap.invalid:389"],
					"bindDn": ["cn=admin,dc=example,dc=com"],
					"usersDn": ["ou=users,dc=example,dc=com"],
					"usernameLDAPAttribute": ["uid"],
					"rdnLDAPAttribute": ["uid"],
					"uuidLDAPAttribute": ["entryUUID"],
					"userObjectClasses": ["inetOrgPerson"],
					"editMode": ["READ_ONLY"]
				}
			}`),
		},
	}
	createAndWaitReady(t, ldap)

	createAndWaitReady(t, &keycloakv1beta1.KeycloakComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "src-" + ldapName + "-mapper", Namespace: testNamespace},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			ParentComponentRef: &keycloakv1beta1.ResourceRef{Name: ldap.Name},
			Name:               strPtr(ldapMapperName),
			Definition: rawJSON(`{
				"providerId": "user-attribute-ldap-mapper",
				"providerType": "org.keycloak.storage.ldap.mappers.LDAPStorageMapper",
				"config": {
					"ldap.attribute": ["departmentNumber"],
					"user.model.attribute": ["department"],
					"read.only": ["true"],
					"always.read.value.from.ldap": ["false"],
					"is.mandatory.in.ldap": ["false"]
				}
			}`),
		},
	})

	// User memberships and client roles via the typed spec fields (#186,
	// #187, #124), plus attributes.
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: user.Name, Namespace: user.Namespace}, user))
	user.Spec.Groups = &[]string{"/" + groupName, "/" + groupName + "/" + childGroupName}
	user.Spec.ClientRoles = &map[string][]string{confClientID: {clientRoleName}}
	userDefWithAttrs := rawJSON(`{"enabled": true, "attributes": {"department": ["engineering"]}}`)
	user.Spec.Definition = &userDefWithAttrs
	require.NoError(t, k8sClient.Update(ctx, user))
	require.NoError(t, wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		groups, err := kc.GetUserGroups(ctx, sourceRealm, mustUserID(t, user))
		if err != nil {
			return false, nil
		}
		return len(groups) == 2, nil
	}), "source user did not get both group memberships")

	targetRealmCR := fmt.Sprintf("rt-dst-%d", suffix)
	targetRealm := createNamedRealmWithOrganizations(t, instanceName, targetRealmCR)

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

	expected := map[string]bool{}
	for _, key := range []string{
		"KeycloakClient/" + clientID,
		"KeycloakClient/" + confClientID,
		"KeycloakClient/" + samlClientID,
		"KeycloakClientScope/" + scopeName,
		"KeycloakGroup/" + groupName,
		"KeycloakGroup/" + groupName + "-" + childGroupName,
		"KeycloakUser/" + userName,
		"KeycloakRole/" + roleName,
		"KeycloakRole/" + shortRoleName,
		"KeycloakRole/" + compositeName,
		"KeycloakRole/" + confClientID + "-" + clientRoleName,
		"KeycloakRoleMapping/" + userName + "-" + roleName,
		"KeycloakRoleMapping/" + userName + "-" + confClientID + "-" + clientRoleName,
		"KeycloakRoleMapping/" + groupName + "-" + shortRoleName,
		"KeycloakRoleMapping/" + groupName + "-" + childGroupName + "-" + shortRoleName,
		"KeycloakProtocolMapper/" + clientID + "-" + mapperName,
		"KeycloakProtocolMapper/" + scopeName + "-" + scopeMapperName,
		"KeycloakOrganization/" + orgName,
		"KeycloakIdentityProvider/" + idpAlias,
		"KeycloakIdentityProviderMapper/" + idpAlias + "-" + idpMapperName,
		"KeycloakComponent/clientregistrationpolicy-" + componentName,
		"KeycloakComponent/" + ldapProviderCR,
		"KeycloakComponent/" + ldapMapperCR,
	} {
		expected[key] = true
	}

	byKey := map[string]export.ExportedResource{}
	for _, res := range resources {
		if expected[res.Kind+"/"+res.Name] {
			byKey[res.Kind+"/"+res.Name] = res
		}
	}
	for key := range expected {
		require.Contains(t, byKey, key, "export did not include %s", key)
	}

	// Omissions do not show up in the re-export comparison (both sides lack
	// the data), so the known omission classes are asserted on the first
	// export directly.
	exportedUser := byKey["KeycloakUser/"+userName].Object.(*keycloakv1beta1.KeycloakUser)
	if assert.NotNil(t, exportedUser.Spec.Groups, "exported user carries no spec.groups (#186)") {
		assert.ElementsMatch(t, []string{"/" + groupName, "/" + groupName + "/" + childGroupName}, *exportedUser.Spec.Groups,
			"exported user memberships (#186, #187)")
	}
	exportedChild := byKey["KeycloakGroup/"+groupName+"-"+childGroupName].Object.(*keycloakv1beta1.KeycloakGroup)
	if assert.NotNil(t, exportedChild.Spec.ParentGroupRef, "exported subgroup has no parentGroupRef (#53)") {
		assert.Equal(t, groupName, exportedChild.Spec.ParentGroupRef.Name)
	}
	assert.Nil(t, exportedChild.Spec.RealmRef, "exported subgroup must not also carry realmRef")
	exportedMapper := byKey["KeycloakComponent/"+ldapMapperCR].Object.(*keycloakv1beta1.KeycloakComponent)
	if assert.NotNil(t, exportedMapper.Spec.ParentComponentRef, "exported LDAP mapper has no parentComponentRef (#167)") {
		assert.Equal(t, ldapProviderCR, exportedMapper.Spec.ParentComponentRef.Name)
	}
	assert.Nil(t, exportedMapper.Spec.RealmRef, "exported LDAP mapper must not also carry realmRef")
	exportedClientRole := byKey["KeycloakRole/"+confClientID+"-"+clientRoleName].Object.(*keycloakv1beta1.KeycloakRole)
	if assert.NotNil(t, exportedClientRole.Spec.ClientRef, "exported client role has no clientRef") {
		assert.Equal(t, confClientID, exportedClientRole.Spec.ClientRef.Name)
	}
	exportedComposite := byKey["KeycloakRole/"+compositeName].Object.(*keycloakv1beta1.KeycloakRole)
	var compositeDef struct {
		Composite  bool `json:"composite"`
		Composites *struct {
			Realm  []string            `json:"realm"`
			Client map[string][]string `json:"client"`
		} `json:"composites"`
	}
	require.NoError(t, json.Unmarshal(exportedComposite.Spec.Definition.Raw, &compositeDef))
	assert.True(t, compositeDef.Composite, "exported composite role lost composite=true")
	if assert.NotNil(t, compositeDef.Composites, "exported composite role carries no composites (#73)") {
		assert.ElementsMatch(t, []string{roleName}, compositeDef.Composites.Realm, "composite realm members")
		assert.Equal(t, map[string][]string{confClientID: {clientRoleName}}, compositeDef.Composites.Client, "composite client members")
	}

	// Snapshot the first export now: the objects below are handed to the API
	// server and later re-read, which fills in metadata and status.
	firstExport := exportedAsJSON(t, resources, expected)

	var applied []client.Object
	for key, res := range byKey {
		obj, ok := res.Object.(client.Object)
		require.True(t, ok, "exported %s is not a client.Object", key)
		// The export strips the masked bind credential; wire the Secret back
		// in, as the export docs tell users to.
		if comp, isComponent := obj.(*keycloakv1beta1.KeycloakComponent); isComponent && comp.Name == ldapProviderCR {
			comp.Spec.ConfigSecretRef = &keycloakv1beta1.ConfigSecretRef{Name: ldapSecretName}
		}
		require.NoError(t, k8sClient.Create(ctx, obj), "create exported %s", key)
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, obj) })
		applied = append(applied, obj)
	}

	roundTripTimeout := 3 * time.Minute
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

	// Third leg: export the target realm with the same options and compare it
	// against the first export. Applying cleanly and reporting Ready does not
	// prove the export was faithful (a mapper that lost its parent or a user
	// that lost its groups still applies), re-exporting does. The comparison
	// covers the resources that were applied; the KeycloakRealm itself is
	// left out because the target realm was created by hand with a minimal
	// definition.
	secondExporter := export.NewExporter(kc, ctrl.Log.WithName("export-roundtrip-2"), export.ExporterOptions{
		Realm:           targetRealm,
		TargetNamespace: testNamespace,
		InstanceRef:     instanceName,
		RealmRef:        targetRealmCR,
		SkipDefaults:    true,
	})
	second, err := secondExporter.Export(context.Background())
	require.NoError(t, err)
	require.Equal(t, firstExport, exportedAsJSON(t, second, expected),
		"re-export of the applied realm differs from the original export")
}

// exportedAsJSON returns the exported resources named in expected, keyed by
// kind/name, as generic JSON so that require.Equal prints a readable diff. Lists
// of strings are sorted: Keycloak returns fields such as optionalClientScopes
// in arbitrary order, and the controllers compare them as sets too.
func exportedAsJSON(t *testing.T, resources []export.ExportedResource, expected map[string]bool) map[string]map[string]interface{} {
	t.Helper()
	out := map[string]map[string]interface{}{}
	for _, res := range resources {
		key := res.Kind + "/" + res.Name
		if !expected[key] {
			continue
		}
		raw, err := json.Marshal(res.Object)
		require.NoError(t, err, "marshal exported %s", key)
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &m))
		sortStringLists(m)
		out[key] = m
	}
	return out
}

// sortStringLists sorts every list made up entirely of strings, recursively.
func sortStringLists(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		for _, child := range x {
			sortStringLists(child)
		}
	case []interface{}:
		allStrings := len(x) > 0
		for _, item := range x {
			if _, ok := item.(string); !ok {
				allStrings = false
			}
			sortStringLists(item)
		}
		if allStrings {
			sort.Slice(x, func(i, j int) bool { return x[i].(string) < x[j].(string) })
		}
	}
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

func mustUserID(t *testing.T, user *keycloakv1beta1.KeycloakUser) string {
	t.Helper()
	current := &keycloakv1beta1.KeycloakUser{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: user.Name, Namespace: user.Namespace}, current))
	require.NotEmpty(t, current.Status.UserID)
	return current.Status.UserID
}
