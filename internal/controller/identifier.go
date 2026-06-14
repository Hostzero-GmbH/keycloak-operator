package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IdentifierMismatchReason is the event/condition reason used when a CR supplies
// the resource identifier in both its first-class spec field and inside
// spec.definition with differing values.
const IdentifierMismatchReason = "IdentifierMismatch"

// resolveIdentifier computes the resolved resource identifier using the
// precedence spec.<id> > definition.<id> > metadata.name. The metadata.name
// fallback is permanent, so an identifier is always derivable.
//
// It returns the resolved value and whether the spec field and the in-definition
// key both supplied a non-empty value that disagree. In Phase 1 a mismatch is a
// soft warning (spec wins, reconcile continues); Phase 2 will reject it.
func resolveIdentifier(specVal *string, defVal, metaName string) (resolved string, mismatch bool) {
	spec := ""
	if specVal != nil {
		spec = *specVal
	}

	switch {
	case spec != "":
		resolved = spec
		mismatch = defVal != "" && defVal != spec
	case defVal != "":
		resolved = defVal
	default:
		resolved = metaName
	}
	return resolved, mismatch
}

// warnIdentifierMismatch surfaces a soft warning when the first-class spec field
// and the in-definition identifier key disagree. The spec value wins and the
// reconcile continues (Phase 1 semantics); the in-definition identifier key is
// deprecated and will be rejected in a future release (Phase 2, which will also
// surface this as a Ready=False condition + Warning event).
func warnIdentifierMismatch(ctx context.Context, field, resolved, defVal string) {
	log.FromContext(ctx).Info("identifier mismatch: spec field overrides differing definition key (the in-definition identifier is deprecated and will be rejected in a future release)",
		"reason", IdentifierMismatchReason, "field", field, "resolved", resolved, "definition", defVal)
}
