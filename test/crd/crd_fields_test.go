// Package crd contains schema-level assertions over the generated CRD YAML in
// config/crd/bases. They guard the identifier contract: every affected CRD must
// expose an identifier spec field, a printer column sourced from the status
// identifier, and an immutability CEL transition rule for the identifier.
// The test reads the generated YAML so it stays cheap and requires no apiserver.
package crd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

const crdDir = "../../config/crd/bases"

type crdExpectation struct {
	file           string
	specField      string // identifier spec property
	columnJSONPath string // additionalPrinterColumns entry sourced from status
	// conditional marks identifiers that are not unconditionally required at the
	// schema level but are instead guarded by a spec-level CEL rule. KeycloakUser
	// uses this: username is required for regular users but omitted for service
	// account users (identified by clientRef), so it cannot be in spec.required.
	conditional bool
}

var expectations = []crdExpectation{
	{file: "keycloak.hostzero.com_keycloakclients.yaml", specField: "clientId", columnJSONPath: ".status.clientId"},
	{file: "keycloak.hostzero.com_keycloakrealms.yaml", specField: "realmName", columnJSONPath: ".status.realmName"},
	{file: "keycloak.hostzero.com_clusterkeycloakrealms.yaml", specField: "realmName", columnJSONPath: ".status.realmName"},
	{file: "keycloak.hostzero.com_keycloakroles.yaml", specField: "name", columnJSONPath: ".status.roleName"},
	{file: "keycloak.hostzero.com_keycloakgroups.yaml", specField: "name", columnJSONPath: ".status.groupName"},
	{file: "keycloak.hostzero.com_keycloakclientscopes.yaml", specField: "name", columnJSONPath: ".status.clientScopeName"},
	{file: "keycloak.hostzero.com_keycloakusers.yaml", specField: "username", columnJSONPath: ".status.username", conditional: true},
	{file: "keycloak.hostzero.com_keycloakorganizations.yaml", specField: "name", columnJSONPath: ".status.organizationName"},
	{file: "keycloak.hostzero.com_keycloakidentityproviders.yaml", specField: "alias", columnJSONPath: ".status.alias"},
	{file: "keycloak.hostzero.com_keycloakidentityprovidermappers.yaml", specField: "name", columnJSONPath: ".status.mapperName"},
	{file: "keycloak.hostzero.com_keycloakprotocolmappers.yaml", specField: "name", columnJSONPath: ".status.mapperName"},
	{file: "keycloak.hostzero.com_keycloakrequiredactions.yaml", specField: "alias", columnJSONPath: ".status.alias"},
	{file: "keycloak.hostzero.com_keycloakcomponents.yaml", specField: "name", columnJSONPath: ".status.componentName"},
}

func loadCRD(t *testing.T, file string) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(crdDir, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("unmarshal %s: %v", file, err)
	}
	return &crd
}

func storageVersion(t *testing.T, crd *apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinitionVersion {
	t.Helper()
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			return v
		}
	}
	t.Fatalf("%s has no storage version", crd.Name)
	return apiextensionsv1.CustomResourceDefinitionVersion{}
}

func TestIdentifierSpecFieldPresent(t *testing.T) {
	for _, exp := range expectations {
		t.Run(exp.file, func(t *testing.T) {
			crd := loadCRD(t, exp.file)
			v := storageVersion(t, crd)
			spec, ok := v.Schema.OpenAPIV3Schema.Properties["spec"]
			if !ok {
				t.Fatalf("%s: no spec in schema", exp.file)
			}
			if _, ok := spec.Properties[exp.specField]; !ok {
				t.Errorf("%s: spec.%s not present in CRD schema", exp.file, exp.specField)
			}
		})
	}
}

func TestIdentifierSpecFieldRequired(t *testing.T) {
	for _, exp := range expectations {
		t.Run(exp.file, func(t *testing.T) {
			crd := loadCRD(t, exp.file)
			v := storageVersion(t, crd)
			spec, ok := v.Schema.OpenAPIV3Schema.Properties["spec"]
			if !ok {
				t.Fatalf("%s: no spec in schema", exp.file)
			}

			if exp.conditional {
				// The identifier is guarded by a spec-level CEL rule rather than
				// spec.required, since it is only required for some shapes.
				found := false
				for _, rule := range spec.XValidations {
					if strings.Contains(rule.Rule, "self."+exp.specField) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: spec.%s must be guarded by a spec-level CEL rule referencing self.%s", exp.file, exp.specField, exp.specField)
				}
			} else {
				required := false
				for _, r := range spec.Required {
					if r == exp.specField {
						required = true
						break
					}
				}
				if !required {
					t.Errorf("%s: spec.%s must be listed as required", exp.file, exp.specField)
				}
			}

			prop, ok := spec.Properties[exp.specField]
			if !ok {
				t.Fatalf("%s: spec.%s not present in CRD schema", exp.file, exp.specField)
			}
			if prop.MinLength == nil || *prop.MinLength < 1 {
				t.Errorf("%s: spec.%s must enforce MinLength>=1 to reject empty identifiers", exp.file, exp.specField)
			}
		})
	}
}

func TestIdentifierPrinterColumnPresent(t *testing.T) {
	for _, exp := range expectations {
		t.Run(exp.file, func(t *testing.T) {
			crd := loadCRD(t, exp.file)
			v := storageVersion(t, crd)
			found := false
			for _, col := range v.AdditionalPrinterColumns {
				if col.JSONPath == exp.columnJSONPath {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: no additionalPrinterColumns entry with jsonPath %q", exp.file, exp.columnJSONPath)
			}
		})
	}
}

// refContract declares, for one schema object, what every *Ref property in it is
// for. It exists so that adding a reference to any CRD without deciding how it
// interacts with the existing ones fails here rather than reaching users: #129
// was a clientRef that silently required a realmRef beside it.
type refContract struct {
	file string
	// path locates the object inside the storage version's spec. Empty means spec
	// itself; {"subject"} means spec.subject.
	path []string
	// exclusive refs participate in a "set exactly one" choice, so every one of
	// them must be named by a CEL rule on the object.
	exclusive []string
	// sole refs are the object's only parent and must therefore be required.
	sole []string
	// data refs address something other than a parent (a Secret, typically) and
	// carry no exclusivity obligation.
	data []string
}

var refContracts = []refContract{
	{file: "keycloak.hostzero.com_clusterkeycloakinstances.yaml"},
	{file: "keycloak.hostzero.com_keycloakinstances.yaml"},

	// Realms attach to an instance.
	{
		file:      "keycloak.hostzero.com_keycloakrealms.yaml",
		exclusive: []string{"instanceRef", "clusterInstanceRef"},
		data:      []string{"smtpSecretRef"},
	},
	{
		file:      "keycloak.hostzero.com_clusterkeycloakrealms.yaml",
		exclusive: []string{"instanceRef", "clusterInstanceRef"},
		data:      []string{"smtpSecretRef"},
	},

	// Realm-scoped kinds with no other possible parent.
	{
		file:      "keycloak.hostzero.com_keycloakauthenticationflows.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakclients.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
		data:      []string{"clientSecretRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakclientscopes.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakcomponents.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
		data:      []string{"configSecretRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakidentityproviders.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
		data:      []string{"configSecretRef", "organizationRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakorganizations.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakrequiredactions.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef"},
		data:      []string{"configSecretRef"},
	},

	// Kinds whose parent can imply the realm, so the parent ref replaces the realm
	// ref instead of accompanying it.
	{
		file:      "keycloak.hostzero.com_keycloakroles.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef", "clientRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakusers.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef", "clientRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakgroups.yaml",
		exclusive: []string{"realmRef", "clusterRealmRef", "parentGroupRef"},
	},

	// Kinds that always derive the realm from a parent and so carry no realm ref.
	{
		file:      "keycloak.hostzero.com_keycloakprotocolmappers.yaml",
		exclusive: []string{"clientRef", "clientScopeRef"},
		data:      []string{"configSecretRef"},
	},
	{
		file: "keycloak.hostzero.com_keycloakidentityprovidermappers.yaml",
		sole: []string{"identityProviderRef"},
		data: []string{"configSecretRef"},
	},
	{
		file: "keycloak.hostzero.com_keycloakusercredentials.yaml",
		sole: []string{"userRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakrolemappings.yaml",
		exclusive: []string{"roleRef"},
	},
	{
		file:      "keycloak.hostzero.com_keycloakrolemappings.yaml",
		path:      []string{"subject"},
		exclusive: []string{"userRef", "groupRef", "serviceAccountRef"},
	},
}

func (c refContract) name() string {
	if len(c.path) == 0 {
		return c.file + " spec"
	}
	return c.file + " spec." + strings.Join(c.path, ".")
}

// resolveObject walks path down from spec.
func resolveObject(t *testing.T, c refContract) apiextensionsv1.JSONSchemaProps {
	t.Helper()
	crd := loadCRD(t, c.file)
	v := storageVersion(t, crd)
	obj, ok := v.Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		t.Fatalf("%s: no spec in schema", c.file)
	}
	for _, p := range c.path {
		next, ok := obj.Properties[p]
		if !ok {
			t.Fatalf("%s: property %q not found", c.name(), p)
		}
		obj = next
	}
	return obj
}

// TestRefContractsAreComplete fails when a schema grows a *Ref property that no
// contract accounts for, which is the drift that lets a new reference ship without
// anyone deciding whether it replaces or accompanies the existing ones.
func TestRefContractsAreComplete(t *testing.T) {
	declared := map[string]map[string]bool{}
	for _, c := range refContracts {
		key := c.name()
		if declared[key] == nil {
			declared[key] = map[string]bool{}
		}
		for _, refs := range [][]string{c.exclusive, c.sole, c.data} {
			for _, ref := range refs {
				declared[key][ref] = true
			}
		}
	}

	for _, c := range refContracts {
		t.Run(c.name(), func(t *testing.T) {
			obj := resolveObject(t, c)
			for prop := range obj.Properties {
				if !strings.HasSuffix(prop, "Ref") {
					continue
				}
				if !declared[c.name()][prop] {
					t.Errorf("%s: %s is not declared in its refContract; classify it as exclusive, sole, or data", c.name(), prop)
				}
			}
		})
	}
}

// TestExclusiveRefsAreConstrained asserts every ref in an exclusive set is named
// by a CEL rule, so it cannot be combined freely with its siblings.
func TestExclusiveRefsAreConstrained(t *testing.T) {
	for _, c := range refContracts {
		if len(c.exclusive) == 0 {
			continue
		}
		t.Run(c.name(), func(t *testing.T) {
			obj := resolveObject(t, c)
			for _, ref := range c.exclusive {
				if _, ok := obj.Properties[ref]; !ok {
					t.Errorf("%s: %s not present in schema", c.name(), ref)
					continue
				}
				found := false
				for _, rule := range obj.XValidations {
					if strings.Contains(rule.Rule, "self."+ref) && !strings.Contains(rule.Rule, "oldSelf") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: no CEL rule constrains %s, so it can be combined with the other refs", c.name(), ref)
				}
			}
		})
	}
}

// TestSoleRefsAreRequired asserts a lone parent ref is structurally required
// rather than left optional with no rule to enforce it.
func TestSoleRefsAreRequired(t *testing.T) {
	for _, c := range refContracts {
		if len(c.sole) == 0 {
			continue
		}
		t.Run(c.name(), func(t *testing.T) {
			obj := resolveObject(t, c)
			for _, ref := range c.sole {
				required := false
				for _, r := range obj.Required {
					if r == ref {
						required = true
						break
					}
				}
				if !required {
					t.Errorf("%s: %s is the only parent ref and must be listed as required", c.name(), ref)
				}
			}
		})
	}
}

func TestIdentifierImmutabilityRule(t *testing.T) {
	for _, exp := range expectations {
		t.Run(exp.file, func(t *testing.T) {
			crd := loadCRD(t, exp.file)
			v := storageVersion(t, crd)
			spec := v.Schema.OpenAPIV3Schema.Properties["spec"]
			found := false
			for _, rule := range spec.XValidations {
				if strings.Contains(rule.Rule, "oldSelf."+exp.specField) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: spec is missing the %s immutability CEL transition rule", exp.file, exp.specField)
			}
		})
	}
}
