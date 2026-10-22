package v1beta1

import "sort"

// RealmNestedCollections maps the RealmRepresentation keys that Keycloak only
// imports when a realm is created to the CRD that owns that datum.
//
// GET /admin/realms/{realm} leaves these out ("will not include nested
// information") and PUT ignores them ("will only update top-level attributes"),
// so neither the KeycloakRealm controller nor the exporter can treat them as
// part of the realm: the controller rejects them in spec.definition, and the
// exporter strips them from an exported realm (older Keycloak versions, before
// 26, still inlined identityProviders and identityProviderMappers on GET).
var RealmNestedCollections = map[string]string{
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

// RealmNestedCollectionKeys returns the keys of RealmNestedCollections in
// sorted order.
func RealmNestedCollectionKeys() []string {
	keys := make([]string, 0, len(RealmNestedCollections))
	for key := range RealmNestedCollections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
