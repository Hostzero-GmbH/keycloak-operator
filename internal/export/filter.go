package export

import "strings"

// Resource type constants
const (
	ResourceTypeRealm             = "realm"
	ResourceTypeClients           = "clients"
	ResourceTypeClientScopes      = "client-scopes"
	ResourceTypeUsers             = "users"
	ResourceTypeGroups            = "groups"
	ResourceTypeRoles             = "roles"
	ResourceTypeRoleMappings      = "role-mappings"
	ResourceTypeIdentityProviders = "identity-providers"
	ResourceTypeComponents        = "components"
	ResourceTypeProtocolMappers   = "protocol-mappers"
	ResourceTypeOrganizations     = "organizations"
)

// Default Keycloak built-in clients to skip
var defaultClients = map[string]bool{
	"account":                     true,
	"account-console":             true,
	"admin-cli":                   true,
	"broker":                      true,
	"realm-management":            true,
	"security-admin-console":      true,
	"master-realm":                true, // Only in master realm
	"{realm}-realm":               true, // Pattern for other realms
}

// Default Keycloak built-in client scopes to skip
var defaultClientScopes = map[string]bool{
	"address":          true,
	"email":            true,
	"microprofile-jwt": true,
	"offline_access":   true,
	"phone":            true,
	"profile":          true,
	"roles":            true,
	"web-origins":      true,
	"acr":              true,
	"basic":            true,
}

// Default Keycloak built-in roles to skip
var defaultRealmRoles = map[string]bool{
	"default-roles-{realm}":  true, // Pattern
	"offline_access":         true,
	"uma_authorization":      true,
	"create-realm":           true, // Only in master
	"admin":                  true, // Only in master
}

// Default Keycloak built-in client roles to skip
var defaultClientRoles = map[string]bool{
	"manage-account":            true,
	"manage-account-links":      true,
	"view-profile":              true,
	"view-consent":              true,
	"manage-consent":            true,
	"delete-account":            true,
	"read-token":                true,
	"view-users":                true,
	"view-clients":              true,
	"view-realm":                true,
	"view-identity-providers":   true,
	"view-events":               true,
	"view-authorization":        true,
	"manage-users":              true,
	"manage-clients":            true,
	"manage-realm":              true,
	"manage-identity-providers": true,
	"manage-events":             true,
	"manage-authorization":      true,
	"query-users":               true,
	"query-clients":             true,
	"query-realms":              true,
	"query-groups":              true,
	"impersonation":             true,
	"create-client":             true,
}

// Default Keycloak built-in users to skip (service accounts)
var defaultUsers = map[string]bool{
	// Service account users are prefixed with "service-account-"
}

// Default component provider types to skip
var defaultComponentProviderTypes = map[string]bool{
	// Skip built-in key providers by default
	"org.keycloak.keys.KeyProvider": true,
}

// Default protocol mappers to skip
var defaultProtocolMappers = map[string]bool{
	// Built-in protocol mappers
	"audience resolve":                    true,
	"client roles":                        true,
	"realm roles":                         true,
	"allowed web origins":                 true,
	"email":                               true,
	"email verified":                      true,
	"family name":                         true,
	"full name":                           true,
	"given name":                          true,
	"groups":                              true,
	"locale":                              true,
	"phone number":                        true,
	"phone number verified":               true,
	"profile":                             true,
	"acr loa level":                       true,
	"address":                             true,
	"nickname":                            true,
	"picture":                             true,
	"preferred_username":                  true,
	"updated at":                          true,
	"username":                            true,
	"website":                             true,
	"zoneinfo":                            true,
	"upn":                                 true,
	"birthdate":                           true,
	"gender":                              true,
	"middle name":                         true,
	"audience":                            true,
	"role list":                           true,
}

// Filter determines which resources to include/exclude
type Filter struct {
	include      map[string]bool
	exclude      map[string]bool
	skipDefaults bool
}

// NewFilter creates a new filter
func NewFilter(include, exclude []string, skipDefaults bool) *Filter {
	f := &Filter{
		include:      make(map[string]bool),
		exclude:      make(map[string]bool),
		skipDefaults: skipDefaults,
	}

	for _, t := range include {
		f.include[strings.ToLower(t)] = true
	}
	for _, t := range exclude {
		f.exclude[strings.ToLower(t)] = true
	}

	return f
}

// ShouldIncludeType checks if a resource type should be included
func (f *Filter) ShouldIncludeType(resourceType string) bool {
	resourceType = strings.ToLower(resourceType)

	// If exclude list is set, check it
	if f.exclude[resourceType] {
		return false
	}

	// If include list is set, only include specified types
	if len(f.include) > 0 {
		return f.include[resourceType]
	}

	return true
}

// ShouldSkipClient checks if a client should be skipped
func (f *Filter) ShouldSkipClient(clientID string) bool {
	if !f.skipDefaults {
		return false
	}

	// Check exact match
	if defaultClients[clientID] {
		return true
	}

	// Check pattern match for realm-specific clients
	if strings.HasSuffix(clientID, "-realm") {
		return true
	}

	return false
}

// ShouldSkipClientScope checks if a client scope should be skipped
func (f *Filter) ShouldSkipClientScope(name string) bool {
	if !f.skipDefaults {
		return false
	}

	return defaultClientScopes[name]
}

// ShouldSkipRole checks if a role should be skipped
func (f *Filter) ShouldSkipRole(name string, isClientRole bool) bool {
	if !f.skipDefaults {
		return false
	}

	if isClientRole {
		return defaultClientRoles[name]
	}

	// Check exact match
	if defaultRealmRoles[name] {
		return true
	}

	// Check pattern match for default-roles-{realm}
	if strings.HasPrefix(name, "default-roles-") {
		return true
	}

	return false
}

// ShouldSkipUser checks if a user should be skipped
func (f *Filter) ShouldSkipUser(username string) bool {
	if !f.skipDefaults {
		return false
	}

	// Skip service account users
	if strings.HasPrefix(username, "service-account-") {
		return true
	}

	return defaultUsers[username]
}

// ShouldSkipComponent checks if a component should be skipped
func (f *Filter) ShouldSkipComponent(name, providerType string) bool {
	if !f.skipDefaults {
		return false
	}

	return defaultComponentProviderTypes[providerType]
}

// ShouldSkipProtocolMapper checks if a protocol mapper should be skipped
func (f *Filter) ShouldSkipProtocolMapper(name string) bool {
	if !f.skipDefaults {
		return false
	}

	return defaultProtocolMappers[name]
}
