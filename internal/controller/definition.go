package controller

import (
	"encoding/json"
	"fmt"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
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

// rejectRealmNestedCollections fails reconcile when a realm definition carries
// any key from keycloakv1beta1.RealmNestedCollections: Keycloak imports those
// only at realm creation, omits them on GET and ignores them on PUT, so a realm
// declaring one would re-PUT on every reconcile while the nested edits never
// reach Keycloak (#172). Keys are checked in sorted order so the reported key
// is deterministic.
func rejectRealmNestedCollections(definition []byte) error {
	for _, key := range keycloakv1beta1.RealmNestedCollectionKeys() {
		if err := rejectDefinitionKey(definition, key, keycloakv1beta1.RealmNestedCollections[key]); err != nil {
			return err
		}
	}
	return nil
}
