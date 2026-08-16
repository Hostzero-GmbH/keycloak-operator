// Package export provides functionality to export Keycloak resources to Kubernetes CRD manifests.
package export

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// groupChildrenPageSize is the page size used when paging through
// /admin/realms/{realm}/groups/{id}/children.
const groupChildrenPageSize = 100

// ExporterOptions configures the export behavior
type ExporterOptions struct {
	// Realm to export
	Realm string

	// Target namespace for generated manifests
	TargetNamespace string

	// Instance reference for generated manifests
	InstanceRef string

	// Realm reference for generated manifests (defaults to realm name)
	RealmRef string

	// Include only these resource types (empty means all)
	Include []string

	// Exclude these resource types
	Exclude []string

	// Skip Keycloak built-in resources
	SkipDefaults bool
}

// Exporter exports Keycloak resources to CRD manifests
type Exporter struct {
	client      *keycloak.Client
	log         logr.Logger
	opts        ExporterOptions
	filter      *Filter
	transformer *Transformer
}

// NewExporter creates a new exporter
func NewExporter(client *keycloak.Client, log logr.Logger, opts ExporterOptions) *Exporter {
	// Set defaults
	if opts.RealmRef == "" {
		opts.RealmRef = sanitizeName(opts.Realm)
	}
	if opts.InstanceRef == "" {
		opts.InstanceRef = "keycloak-instance"
	}

	return &Exporter{
		client: client,
		log:    log.WithName("exporter"),
		opts:   opts,
		filter: NewFilter(opts.Include, opts.Exclude, opts.SkipDefaults),
		transformer: NewTransformer(TransformerOptions{
			TargetNamespace: opts.TargetNamespace,
			InstanceRef:     opts.InstanceRef,
			RealmRef:        opts.RealmRef,
		}),
	}
}

// ExportedResource represents an exported Keycloak resource
type ExportedResource struct {
	Kind       string
	Name       string
	APIVersion string
	Object     interface{}
}

// Export exports all resources from the realm
func (e *Exporter) Export(ctx context.Context) ([]ExportedResource, error) {
	var resources []ExportedResource

	// Export in dependency order
	exporters := []struct {
		name     string
		typeName string
		fn       func(ctx context.Context) ([]ExportedResource, error)
	}{
		{"realm", ResourceTypeRealm, e.exportRealm},
		{"client-scopes", ResourceTypeClientScopes, e.exportClientScopes},
		{"clients", ResourceTypeClients, e.exportClients},
		{"groups", ResourceTypeGroups, e.exportGroups},
		{"users", ResourceTypeUsers, e.exportUsers},
		{"realm-roles", ResourceTypeRoles, e.exportRealmRoles},
		{"client-roles", ResourceTypeRoles, e.exportClientRoles},
		{"identity-providers", ResourceTypeIdentityProviders, e.exportIdentityProviders},
		{"components", ResourceTypeComponents, e.exportComponents},
		{"organizations", ResourceTypeOrganizations, e.exportOrganizations},
	}

	for _, exp := range exporters {
		if !e.filter.ShouldIncludeType(exp.typeName) {
			e.log.V(1).Info("Skipping resource type", "type", exp.name)
			continue
		}

		e.log.V(1).Info("Exporting", "type", exp.name)
		res, err := exp.fn(ctx)
		if err != nil {
			// Log error but continue with other resources
			e.log.Error(err, "Failed to export", "type", exp.name)
			continue
		}
		resources = append(resources, res...)
	}

	return resources, nil
}

func (e *Exporter) exportRealm(ctx context.Context) ([]ExportedResource, error) {
	raw, err := e.client.GetRealmRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm: %w", err)
	}

	resource, err := e.transformer.TransformRealm(raw, e.opts.Realm)
	if err != nil {
		return nil, err
	}

	return []ExportedResource{resource}, nil
}

func (e *Exporter) exportClientScopes(ctx context.Context) ([]ExportedResource, error) {
	rawScopes, err := e.client.GetClientScopesRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get client scopes: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawScopes {
		// Parse to check if we should skip
		var scope struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &scope); err != nil {
			continue
		}

		if e.filter.ShouldSkipClientScope(scope.Name) {
			continue
		}

		resource, err := e.transformer.TransformClientScope(raw)
		if err != nil {
			e.log.Error(err, "Failed to transform client scope", "name", scope.Name)
			continue
		}
		resources = append(resources, resource)

		// Export protocol mappers for this scope
		if e.filter.ShouldIncludeType(ResourceTypeProtocolMappers) {
			mappers, err := e.exportClientScopeProtocolMappers(ctx, scope.Name)
			if err != nil {
				e.log.Error(err, "Failed to export protocol mappers for client scope", "scope", scope.Name)
			} else {
				resources = append(resources, mappers...)
			}
		}
	}

	return resources, nil
}

func (e *Exporter) exportClients(ctx context.Context) ([]ExportedResource, error) {
	rawClients, err := e.client.GetClientsRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawClients {
		// Parse to check if we should skip
		var client struct {
			ID       string `json:"id"`
			ClientID string `json:"clientId"`
		}
		if err := json.Unmarshal(raw, &client); err != nil {
			continue
		}

		if e.filter.ShouldSkipClient(client.ClientID) {
			continue
		}

		resource, err := e.transformer.TransformClient(raw, client.ClientID)
		if err != nil {
			e.log.Error(err, "Failed to transform client", "clientId", client.ClientID)
			continue
		}
		resources = append(resources, resource)

		// Export protocol mappers for this client
		if e.filter.ShouldIncludeType(ResourceTypeProtocolMappers) {
			mappers, err := e.exportClientProtocolMappers(ctx, client.ID, client.ClientID)
			if err != nil {
				e.log.Error(err, "Failed to export protocol mappers for client", "clientId", client.ClientID)
			} else {
				resources = append(resources, mappers...)
			}
		}

		// Export client roles
		if e.filter.ShouldIncludeType(ResourceTypeRoles) {
			roles, err := e.exportClientRolesForClient(ctx, client.ID, client.ClientID)
			if err != nil {
				e.log.Error(err, "Failed to export roles for client", "clientId", client.ClientID)
			} else {
				resources = append(resources, roles...)
			}
		}
	}

	return resources, nil
}

// groupHeader captures the fields we need from a raw group response to
// determine its identity and whether it has children to fetch.
type groupHeader struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	SubGroupCount int               `json:"subGroupCount"`
	SubGroups     []json.RawMessage `json:"subGroups"`
}

func (e *Exporter) exportGroups(ctx context.Context) ([]ExportedResource, error) {
	rawGroups, err := e.client.GetGroupsRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawGroups {
		var group groupHeader
		if err := json.Unmarshal(raw, &group); err != nil {
			continue
		}

		resource, err := e.transformer.TransformGroup(raw, "")
		if err != nil {
			e.log.Error(err, "Failed to transform group", "name", group.Name)
			continue
		}
		resources = append(resources, resource)

		children := e.resolveGroupChildren(ctx, group)
		subgroups := e.exportSubGroups(ctx, children, group.Name)
		resources = append(resources, subgroups...)

		if e.filter.ShouldIncludeType(ResourceTypeRoleMappings) {
			mappings, err := e.exportGroupRoleMappings(ctx, group.ID, group.Name)
			if err != nil {
				e.log.Error(err, "Failed to export role mappings for group", "name", group.Name)
			} else {
				resources = append(resources, mappings...)
			}
		}
	}

	return resources, nil
}

func (e *Exporter) exportSubGroups(ctx context.Context, subgroups []json.RawMessage, parentName string) []ExportedResource {
	var resources []ExportedResource
	for _, raw := range subgroups {
		var group groupHeader
		if err := json.Unmarshal(raw, &group); err != nil {
			continue
		}

		resource, err := e.transformer.TransformGroup(raw, parentName)
		if err != nil {
			e.log.Error(err, "Failed to transform subgroup", "name", group.Name, "parent", parentName)
			continue
		}
		resources = append(resources, resource)

		fullPath := parentName + "/" + group.Name

		if e.filter.ShouldIncludeType(ResourceTypeRoleMappings) && group.ID != "" {
			mappings, err := e.exportGroupRoleMappings(ctx, group.ID, fullPath)
			if err != nil {
				e.log.Error(err, "Failed to export role mappings for subgroup", "name", group.Name, "parent", parentName)
			} else {
				resources = append(resources, mappings...)
			}
		}

		children := e.resolveGroupChildren(ctx, group)
		subres := e.exportSubGroups(ctx, children, fullPath)
		resources = append(resources, subres...)
	}
	return resources
}

// resolveGroupChildren returns the direct children of a group. It prefers the
// inline subGroups field (legacy Keycloak < 23) and falls back to the
// /groups/{id}/children endpoint when the listing returned an empty subGroups
// array but a non-zero subGroupCount (Keycloak 23+).
func (e *Exporter) resolveGroupChildren(ctx context.Context, group groupHeader) []json.RawMessage {
	if len(group.SubGroups) > 0 {
		return group.SubGroups
	}
	if group.SubGroupCount <= 0 || group.ID == "" {
		return nil
	}

	children, err := e.listGroupChildren(ctx, group.ID)
	if err != nil {
		e.log.Error(err, "Failed to list group children", "name", group.Name, "id", group.ID)
		return nil
	}
	return children
}

// listGroupChildren paginates through /admin/realms/{realm}/groups/{id}/children
// and returns all children as raw JSON.
func (e *Exporter) listGroupChildren(ctx context.Context, groupID string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	for offset := 0; ; offset += groupChildrenPageSize {
		page, err := e.client.GetGroupChildrenRaw(ctx, e.opts.Realm, groupID, map[string]string{
			"first":               strconv.Itoa(offset),
			"max":                 strconv.Itoa(groupChildrenPageSize),
			"briefRepresentation": "false",
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < groupChildrenPageSize {
			return all, nil
		}
	}
}

func (e *Exporter) exportUsers(ctx context.Context) ([]ExportedResource, error) {
	// Get all users with pagination
	rawUsers, err := e.client.GetUsersRaw(ctx, e.opts.Realm, map[string]string{"max": "1000"})
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawUsers {
		var user struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(raw, &user); err != nil {
			continue
		}

		if e.filter.ShouldSkipUser(user.Username) {
			continue
		}

		resource, err := e.transformer.TransformUser(raw)
		if err != nil {
			e.log.Error(err, "Failed to transform user", "username", user.Username)
			continue
		}
		resources = append(resources, resource)

		// Export role mappings for this user
		if e.filter.ShouldIncludeType(ResourceTypeRoleMappings) {
			mappings, err := e.exportUserRoleMappings(ctx, user.ID, user.Username)
			if err != nil {
				e.log.Error(err, "Failed to export role mappings for user", "username", user.Username)
			} else {
				resources = append(resources, mappings...)
			}
		}
	}

	return resources, nil
}

func (e *Exporter) exportRealmRoles(ctx context.Context) ([]ExportedResource, error) {
	rawRoles, err := e.client.GetRealmRolesRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm roles: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawRoles {
		var role struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &role); err != nil {
			continue
		}

		if e.filter.ShouldSkipRole(role.Name, false) {
			continue
		}

		resource, err := e.transformer.TransformRole(raw, "", "")
		if err != nil {
			e.log.Error(err, "Failed to transform realm role", "name", role.Name)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportClientRoles(ctx context.Context) ([]ExportedResource, error) {
	// Client roles are exported per-client in exportClients
	return nil, nil
}

func (e *Exporter) exportClientRolesForClient(ctx context.Context, clientUUID, clientID string) ([]ExportedResource, error) {
	rawRoles, err := e.client.GetClientRolesRaw(ctx, e.opts.Realm, clientUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client roles: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawRoles {
		var role struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &role); err != nil {
			continue
		}

		if e.filter.ShouldSkipRole(role.Name, true) {
			continue
		}

		resource, err := e.transformer.TransformRole(raw, clientID, clientUUID)
		if err != nil {
			e.log.Error(err, "Failed to transform client role", "name", role.Name, "client", clientID)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportIdentityProviders(ctx context.Context) ([]ExportedResource, error) {
	rawIDPs, err := e.client.GetIdentityProvidersRaw(ctx, e.opts.Realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity providers: %w", err)
	}

	e.transformer.SetOrganizationNames(e.loadOrganizationNames(ctx))

	var resources []ExportedResource
	for _, raw := range rawIDPs {
		var idp struct {
			Alias          string `json:"alias"`
			OrganizationID string `json:"organizationId"`
		}
		if err := json.Unmarshal(raw, &idp); err != nil {
			continue
		}

		resource, err := e.transformer.TransformIdentityProvider(raw)
		if err != nil {
			e.log.Error(err, "Failed to transform identity provider", "alias", idp.Alias)
			continue
		}
		if idp.OrganizationID != "" {
			if exported, ok := resource.Object.(*keycloakv1beta1.KeycloakIdentityProvider); !ok || exported.Spec.OrganizationRef == nil {
				e.log.Info("dropping unresolved organizationId on identity provider", "alias", idp.Alias, "organizationId", idp.OrganizationID)
			}
		}
		resources = append(resources, resource)

		if e.filter.ShouldIncludeType(ResourceTypeIdentityProviderMappers) {
			mappers, err := e.exportIdentityProviderMappers(ctx, idp.Alias)
			if err != nil {
				e.log.Error(err, "Failed to export identity provider mappers", "alias", idp.Alias)
			} else {
				resources = append(resources, mappers...)
			}
		}
	}

	return resources, nil
}

func (e *Exporter) exportIdentityProviderMappers(ctx context.Context, alias string) ([]ExportedResource, error) {
	rawMappers, err := e.client.GetIdentityProviderMappersRaw(ctx, e.opts.Realm, alias)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity provider mappers: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawMappers {
		var mapper struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &mapper); err != nil {
			continue
		}

		resource, err := e.transformer.TransformIdentityProviderMapper(raw, alias)
		if err != nil {
			e.log.Error(err, "Failed to transform identity provider mapper", "alias", alias, "name", mapper.Name)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportComponents(ctx context.Context) ([]ExportedResource, error) {
	rawComponents, err := e.client.GetComponentsRaw(ctx, e.opts.Realm, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get components: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawComponents {
		var component struct {
			Name         string `json:"name"`
			ProviderType string `json:"providerType"`
		}
		if err := json.Unmarshal(raw, &component); err != nil {
			continue
		}

		if e.filter.ShouldSkipComponent(component.Name, component.ProviderType) {
			continue
		}

		resource, err := e.transformer.TransformComponent(raw)
		if err != nil {
			e.log.Error(err, "Failed to transform component", "name", component.Name)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

// loadOrganizationNames fetches organizations independently of include/exclude
// filters so identity-provider export can resolve organizationRef even when
// organizations themselves are not being exported.
func (e *Exporter) loadOrganizationNames(ctx context.Context) map[string]string {
	rawOrgs, err := e.client.GetOrganizationsRaw(ctx, e.opts.Realm)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			return nil
		}
		e.log.Error(err, "Failed to list organizations for identity provider organizationRef resolution")
		return nil
	}

	names := make(map[string]string, len(rawOrgs))
	for _, raw := range rawOrgs {
		var org struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &org); err != nil || org.ID == "" || org.Name == "" {
			continue
		}
		names[org.ID] = sanitizeName(org.Name)
	}
	return names
}

func (e *Exporter) exportOrganizations(ctx context.Context) ([]ExportedResource, error) {
	rawOrgs, err := e.client.GetOrganizationsRaw(ctx, e.opts.Realm)
	if err != nil {
		// Organizations might not be enabled or supported in this Keycloak version
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			e.log.V(1).Info("Organizations not available (requires Keycloak 26+)")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organizations: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawOrgs {
		var org struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &org); err != nil {
			continue
		}

		resource, err := e.transformer.TransformOrganization(raw)
		if err != nil {
			e.log.Error(err, "Failed to transform organization", "name", org.Name)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportClientProtocolMappers(ctx context.Context, clientUUID, clientID string) ([]ExportedResource, error) {
	rawMappers, err := e.client.GetClientProtocolMappersRaw(ctx, e.opts.Realm, clientUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol mappers: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawMappers {
		var mapper struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &mapper); err != nil {
			continue
		}

		if e.filter.ShouldSkipProtocolMapper(mapper.Name) {
			continue
		}

		resource, err := e.transformer.TransformProtocolMapper(raw, clientID, "")
		if err != nil {
			e.log.Error(err, "Failed to transform protocol mapper", "name", mapper.Name, "client", clientID)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportClientScopeProtocolMappers(ctx context.Context, scopeName string) ([]ExportedResource, error) {
	// Get scope ID first
	scope, err := e.client.GetClientScopeByName(ctx, e.opts.Realm, scopeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client scope: %w", err)
	}

	rawMappers, err := e.client.GetClientScopeProtocolMappersRaw(ctx, e.opts.Realm, *scope.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol mappers: %w", err)
	}

	var resources []ExportedResource
	for _, raw := range rawMappers {
		var mapper struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &mapper); err != nil {
			continue
		}

		if e.filter.ShouldSkipProtocolMapper(mapper.Name) {
			continue
		}

		resource, err := e.transformer.TransformProtocolMapper(raw, "", scopeName)
		if err != nil {
			e.log.Error(err, "Failed to transform protocol mapper", "name", mapper.Name, "scope", scopeName)
			continue
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (e *Exporter) exportUserRoleMappings(ctx context.Context, userID, username string) ([]ExportedResource, error) {
	var resources []ExportedResource

	// Get realm role mappings
	realmRoles, err := e.client.GetUserRealmRoleMappings(ctx, e.opts.Realm, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user realm role mappings: %w", err)
	}

	for _, role := range realmRoles {
		if role.Name == nil {
			continue
		}
		if e.filter.ShouldSkipRole(*role.Name, false) {
			continue
		}

		resource, err := e.transformer.TransformRoleMapping("user", username, *role.Name, "", "")
		if err != nil {
			e.log.Error(err, "Failed to transform role mapping", "user", username, "role", *role.Name)
			continue
		}
		resources = append(resources, resource)
	}

	// Get client role mappings for each client
	clients, err := e.client.GetClients(ctx, e.opts.Realm, nil)
	if err != nil {
		return resources, nil // Continue with what we have
	}

	for _, client := range clients {
		if client.ID == nil || client.ClientID == nil {
			continue
		}

		clientRoles, err := e.client.GetUserClientRoleMappings(ctx, e.opts.Realm, userID, *client.ID)
		if err != nil {
			continue
		}

		for _, role := range clientRoles {
			if role.Name == nil {
				continue
			}
			if e.filter.ShouldSkipRole(*role.Name, true) {
				continue
			}

			resource, err := e.transformer.TransformRoleMapping("user", username, *role.Name, *client.ClientID, *client.ID)
			if err != nil {
				e.log.Error(err, "Failed to transform client role mapping", "user", username, "role", *role.Name, "client", *client.ClientID)
				continue
			}
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

func (e *Exporter) exportGroupRoleMappings(ctx context.Context, groupID, groupName string) ([]ExportedResource, error) {
	var resources []ExportedResource

	// Get realm role mappings
	realmRoles, err := e.client.GetGroupRealmRoleMappings(ctx, e.opts.Realm, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group realm role mappings: %w", err)
	}

	for _, role := range realmRoles {
		if role.Name == nil {
			continue
		}
		if e.filter.ShouldSkipRole(*role.Name, false) {
			continue
		}

		resource, err := e.transformer.TransformRoleMapping("group", groupName, *role.Name, "", "")
		if err != nil {
			e.log.Error(err, "Failed to transform role mapping", "group", groupName, "role", *role.Name)
			continue
		}
		resources = append(resources, resource)
	}

	// Get client role mappings for each client
	clients, err := e.client.GetClients(ctx, e.opts.Realm, nil)
	if err != nil {
		return resources, nil
	}

	for _, client := range clients {
		if client.ID == nil || client.ClientID == nil {
			continue
		}

		clientRoles, err := e.client.GetGroupClientRoleMappings(ctx, e.opts.Realm, groupID, *client.ID)
		if err != nil {
			continue
		}

		for _, role := range clientRoles {
			if role.Name == nil {
				continue
			}
			if e.filter.ShouldSkipRole(*role.Name, true) {
				continue
			}

			resource, err := e.transformer.TransformRoleMapping("group", groupName, *role.Name, *client.ClientID, *client.ID)
			if err != nil {
				e.log.Error(err, "Failed to transform client role mapping", "group", groupName, "role", *role.Name, "client", *client.ClientID)
				continue
			}
			resources = append(resources, resource)
		}
	}

	return resources, nil
}
