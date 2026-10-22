package controller

import (
	"encoding/json"
	"fmt"
	"sort"
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

// realmNestedCollections maps the RealmRepresentation keys that Keycloak only
// imports when a realm is created to the CRD that owns that datum. GET
// /admin/realms/{realm} omits them ("will not include nested information") and
// PUT ignores them ("will only update top-level attributes"), so the operator
// can neither compare nor apply them through the realm: a realm declaring one
// would re-PUT on every reconcile while the nested edits never reach Keycloak.
var realmNestedCollections = map[string]string{
	"authenticationFlows":     "KeycloakAuthenticationFlow",
	"clientScopes":            "KeycloakClientScope",
	"clients":                 "KeycloakClient",
	"components":              "KeycloakComponent",
	"groups":                  "KeycloakGroup",
	"identityProviderMappers": "KeycloakIdentityProviderMapper",
	"identityProviders":       "KeycloakIdentityProvider",
	"organizations":           "KeycloakOrganization",
	"requiredActions":         "KeycloakRequiredAction",
	"roles":                   "KeycloakRole",
	"users":                   "KeycloakUser",
}

// rejectRealmNestedCollections fails reconcile when a realm definition carries
// any key from realmNestedCollections. Keys are checked in sorted order so the
// reported key is deterministic.
func rejectRealmNestedCollections(definition []byte) error {
	keys := make([]string, 0, len(realmNestedCollections))
	for key := range realmNestedCollections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := rejectDefinitionKey(definition, key, realmNestedCollections[key]); err != nil {
			return err
		}
	}
	return nil
}
