package crd

import (
	"encoding/json"
	"testing"
)

// managementPolicyCRDs lists the CRDs that expose spec.managementPolicy.
var managementPolicyCRDs = []string{
	"keycloak.hostzero.com_keycloakrealms.yaml",
	"keycloak.hostzero.com_clusterkeycloakrealms.yaml",
}

// TestManagementPolicyField guards the managementPolicy contract: an optional
// Manage|Observe enum defaulting to Manage, with spec.definition still
// required, and a wide-only printer column.
func TestManagementPolicyField(t *testing.T) {
	for _, file := range managementPolicyCRDs {
		t.Run(file, func(t *testing.T) {
			v := storageVersion(t, loadCRD(t, file))
			spec := v.Schema.OpenAPIV3Schema.Properties["spec"]

			field, ok := spec.Properties["managementPolicy"]
			if !ok {
				t.Fatalf("spec.managementPolicy missing")
			}
			if field.Type != "string" {
				t.Errorf("type = %q, want string", field.Type)
			}

			var enum []string
			for _, e := range field.Enum {
				var s string
				if err := json.Unmarshal(e.Raw, &s); err != nil {
					t.Fatalf("enum value %s: %v", e.Raw, err)
				}
				enum = append(enum, s)
			}
			if len(enum) != 2 || enum[0] != "Manage" || enum[1] != "Observe" {
				t.Errorf("enum = %v, want [Manage Observe]", enum)
			}

			if field.Default == nil || string(field.Default.Raw) != `"Manage"` {
				t.Errorf("default = %v, want \"Manage\"", field.Default)
			}

			required := map[string]bool{}
			for _, r := range spec.Required {
				required[r] = true
			}
			if required["managementPolicy"] {
				t.Errorf("managementPolicy must be optional")
			}
			if !required["definition"] {
				t.Errorf("definition must stay required in every management policy")
			}

			found := false
			for _, col := range v.AdditionalPrinterColumns {
				if col.JSONPath == ".spec.managementPolicy" {
					found = true
					if col.Priority != 1 {
						t.Errorf("Policy column priority = %d, want 1", col.Priority)
					}
				}
			}
			if !found {
				t.Errorf("no printer column for .spec.managementPolicy")
			}
		})
	}
}
