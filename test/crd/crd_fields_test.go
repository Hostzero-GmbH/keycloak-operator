// Package crd contains schema-level assertions over the generated CRD YAML in
// config/crd/bases. They guard the identifier contract: every affected CRD must
// expose an identifier spec field and a printer column sourced from the status
// identifier, and the realm CRDs must carry the realmName immutability CEL rule.
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
	{file: "keycloak.hostzero.com_keycloakclients.yaml", specField: "clientId", columnJSONPath: ".status.clientID"},
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

func TestRealmNameImmutabilityRule(t *testing.T) {
	realmCRDs := []string{
		"keycloak.hostzero.com_keycloakrealms.yaml",
		"keycloak.hostzero.com_clusterkeycloakrealms.yaml",
	}
	for _, file := range realmCRDs {
		t.Run(file, func(t *testing.T) {
			crd := loadCRD(t, file)
			v := storageVersion(t, crd)
			spec := v.Schema.OpenAPIV3Schema.Properties["spec"]
			found := false
			for _, rule := range spec.XValidations {
				if strings.Contains(rule.Rule, "oldSelf.realmName") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: spec is missing the realmName immutability CEL transition rule", file)
			}
		})
	}
}
