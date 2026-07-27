package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InvalidIdentifierReason is the status/condition reason used when a CR's
// resource identifier cannot be resolved from its spec field.
const InvalidIdentifierReason = "InvalidIdentifier"

// resolveIdentifier returns the resource identifier from its required spec field.
// An identifier inside spec.definition is tolerated only when it matches the spec
// value (so pre-existing manifests migrate by adding the spec field alone); a
// conflicting value is an error.
//
// specField is the spec property name (e.g. "realmName") used in error messages.
// defVal is the identifier found in spec.definition, if any.
func resolveIdentifier(specField string, specVal *string, defVal string) (string, error) {
	spec := ""
	if specVal != nil {
		spec = *specVal
	}
	if spec == "" {
		return "", fmt.Errorf("spec.%s is required", specField)
	}
	if defVal != "" && defVal != spec {
		return "", fmt.Errorf("the identifier in spec.definition (%q) conflicts with spec.%s (%q); remove it from the definition", defVal, specField, spec)
	}
	return spec, nil
}

// persistResolvedIdentifier records the resolved identifier in status and
// persists it immediately when it changed. It is needed by reconcilers whose
// updateStatus skips the API write when ready/status/message are unchanged:
// an identifier resolved for the first time on an already-ready resource
// (operator upgrade) would otherwise never reach the API server, and dependent
// controllers read it from status.
func persistResolvedIdentifier(ctx context.Context, c client.Client, obj client.Object, field *string, value string) error {
	if *field == value {
		return nil
	}
	*field = value
	return c.Status().Update(ctx, obj)
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
