// Package crd contains schema-level assertions over the generated CRD YAML in
// config/crd/bases. These guard the identifier-unification contract: every
// affected CRD must expose a first-class identifier spec field and a printer
// column sourced from the resolved status identifier, and the realm CRDs must
// carry the realmName immutability CEL rule. The test reads the generated YAML
// so it stays cheap and requires no apiserver.
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
	specField      string // first-class identifier spec property
	columnJSONPath string // additionalPrinterColumns entry sourced from status
}

var expectations = []crdExpectation{
	{"keycloak.hostzero.com_keycloakclients.yaml", "clientId", ".status.clientID"},
	{"keycloak.hostzero.com_keycloakrealms.yaml", "realmName", ".status.realmName"},
	{"keycloak.hostzero.com_clusterkeycloakrealms.yaml", "realmName", ".status.realmName"},
	{"keycloak.hostzero.com_keycloakroles.yaml", "name", ".status.roleName"},
	{"keycloak.hostzero.com_keycloakgroups.yaml", "name", ".status.groupName"},
	{"keycloak.hostzero.com_keycloakclientscopes.yaml", "name", ".status.clientScopeName"},
	{"keycloak.hostzero.com_keycloakusers.yaml", "username", ".status.username"},
	{"keycloak.hostzero.com_keycloakorganizations.yaml", "name", ".status.organizationName"},
	{"keycloak.hostzero.com_keycloakidentityproviders.yaml", "alias", ".status.alias"},
	{"keycloak.hostzero.com_keycloakidentityprovidermappers.yaml", "name", ".status.mapperName"},
	{"keycloak.hostzero.com_keycloakprotocolmappers.yaml", "name", ".status.mapperName"},
	{"keycloak.hostzero.com_keycloakrequiredactions.yaml", "alias", ".status.alias"},
	{"keycloak.hostzero.com_keycloakcomponents.yaml", "name", ".status.componentName"},
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
