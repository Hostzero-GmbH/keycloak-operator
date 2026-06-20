package controller

import "fmt"

// InvalidIdentifierReason is the status/condition reason used when a CR's
// resource identifier cannot be resolved from its spec field.
const InvalidIdentifierReason = "InvalidIdentifier"

// resolveIdentifier returns the resource identifier from its required spec field.
// The identifier must be set there, not inside spec.definition.
//
// specField is the spec property name (e.g. "realmName") used in error messages.
// defVal is the identifier found in spec.definition, if any; setting it there is
// an error.
func resolveIdentifier(specField string, specVal *string, defVal string) (string, error) {
	spec := ""
	if specVal != nil {
		spec = *specVal
	}
	if defVal != "" {
		return "", fmt.Errorf("the identifier must be set via spec.%s, not inside spec.definition", specField)
	}
	if spec == "" {
		return "", fmt.Errorf("spec.%s is required", specField)
	}
	return spec, nil
}

// identifierValue returns the dereferenced identifier. It is used by secondary
// code paths (deletion, credential writes, drift lookups) that run after the
// main reconcile has already validated the identifier with resolveIdentifier.
func identifierValue(specVal *string) string {
	if specVal == nil {
		return ""
	}
	return *specVal
}
