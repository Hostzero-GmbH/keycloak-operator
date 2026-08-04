package controller

import (
	"encoding/json"
	"fmt"
)

// UnsupportedDefinitionFieldReason is the status/condition reason used when
// spec.definition carries a key whose datum belongs to a dedicated CRD.
const UnsupportedDefinitionFieldReason = "UnsupportedDefinitionField"

// rejectDefinitionKey fails reconcile when spec.definition contains key.
//
// Keycloak manages some sub-resources only through their own endpoints and
// silently ignores them on the parent representation PUT (client scope protocol
// mappers return 204 while discarding the change). Such data has its own CRD, so
// accepting it here would mean a second home for the same datum and, worse, edits
// that appear to succeed but never reach Keycloak.
func rejectDefinitionKey(definition []byte, key, ownerKind string) error {
	if len(definition) == 0 {
		return nil
	}

	// Malformed JSON is reported by the caller's own parse of the definition.
	var defMap map[string]json.RawMessage
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return nil
	}

	if _, present := defMap[key]; !present {
		return nil
	}

	return fmt.Errorf("spec.definition.%s is not supported; declare each entry as a %s resource instead", key, ownerKind)
}
