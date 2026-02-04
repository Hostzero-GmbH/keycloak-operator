package export

import (
	"encoding/json"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// TransformerOptions configures the transformer
type TransformerOptions struct {
	TargetNamespace string
	InstanceRef     string
	RealmRef        string
}

// Transformer transforms Keycloak JSON to CRD structs
type Transformer struct {
	opts TransformerOptions
}

// NewTransformer creates a new transformer
func NewTransformer(opts TransformerOptions) *Transformer {
	return &Transformer{opts: opts}
}

// TransformRealm transforms a realm JSON to KeycloakRealm
func (t *Transformer) TransformRealm(raw json.RawMessage, realmName string) (ExportedResource, error) {
	// Remove server-managed fields
	definition := removeServerFields(raw, "id")

	realm := &keycloakv1beta1.KeycloakRealm{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakRealm",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(realmName),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakRealmSpec{
			InstanceRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.InstanceRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakRealm",
		Name:       realm.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     realm,
	}, nil
}

// TransformClient transforms a client JSON to KeycloakClient
func (t *Transformer) TransformClient(raw json.RawMessage, clientID string) (ExportedResource, error) {
	// Parse client to check if it's confidential
	var parsed struct {
		PublicClient bool `json:"publicClient"`
		BearerOnly   bool `json:"bearerOnly"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields and secrets
	definition := removeServerFields(raw, "id", "secret", "registrationAccessToken")

	client := &keycloakv1beta1.KeycloakClient{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakClient",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(clientID),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakClientSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: &runtime.RawExtension{Raw: definition},
		},
	}

	// Add clientSecretRef for confidential clients (not public, not bearer-only)
	if !parsed.PublicClient && !parsed.BearerOnly {
		client.Spec.ClientSecretRef = &keycloakv1beta1.ClientSecretRefSpec{
			Name: sanitizeName(clientID) + "-secret",
		}
	}

	return ExportedResource{
		Kind:       "KeycloakClient",
		Name:       client.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     client,
	}, nil
}

// TransformClientScope transforms a client scope JSON to KeycloakClientScope
func (t *Transformer) TransformClientScope(raw json.RawMessage) (ExportedResource, error) {
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields
	definition := removeServerFields(raw, "id")

	scope := &keycloakv1beta1.KeycloakClientScope{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakClientScope",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(parsed.Name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakClientScopeSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakClientScope",
		Name:       scope.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     scope,
	}, nil
}

// TransformUser transforms a user JSON to KeycloakUser
func (t *Transformer) TransformUser(raw json.RawMessage) (ExportedResource, error) {
	var parsed struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields and sensitive data
	definition := removeServerFields(raw, "id", "createdTimestamp", "credentials", "federatedIdentities", "access")

	user := &keycloakv1beta1.KeycloakUser{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakUser",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(parsed.Username),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakUserSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: &runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakUser",
		Name:       user.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     user,
	}, nil
}

// TransformGroup transforms a group JSON to KeycloakGroup
func (t *Transformer) TransformGroup(raw json.RawMessage, parentGroupName string) (ExportedResource, error) {
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields and subgroups (exported separately)
	definition := removeServerFields(raw, "id", "subGroups", "path")

	name := parsed.Name
	if parentGroupName != "" {
		name = parentGroupName + "-" + parsed.Name
	}

	group := &keycloakv1beta1.KeycloakGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakGroupSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	// Add parent reference if this is a subgroup
	if parentGroupName != "" {
		group.Spec.ParentGroupRef = &keycloakv1beta1.ResourceRef{
			Name: sanitizeName(parentGroupName),
		}
	}

	return ExportedResource{
		Kind:       "KeycloakGroup",
		Name:       group.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     group,
	}, nil
}

// TransformRole transforms a role JSON to KeycloakRole
func (t *Transformer) TransformRole(raw json.RawMessage, clientID, clientUUID string) (ExportedResource, error) {
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields
	definition := removeServerFields(raw, "id", "containerId")

	name := parsed.Name
	if clientID != "" {
		name = clientID + "-" + parsed.Name
	}

	role := &keycloakv1beta1.KeycloakRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakRoleSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	// Add client reference if this is a client role
	if clientID != "" {
		role.Spec.ClientRef = &keycloakv1beta1.ResourceRef{
			Name: sanitizeName(clientID),
		}
	}

	return ExportedResource{
		Kind:       "KeycloakRole",
		Name:       role.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     role,
	}, nil
}

// TransformIdentityProvider transforms an identity provider JSON to KeycloakIdentityProvider
func (t *Transformer) TransformIdentityProvider(raw json.RawMessage) (ExportedResource, error) {
	var parsed struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove sensitive config fields
	definition := removeServerFields(raw, "internalId", "config.clientSecret")

	idp := &keycloakv1beta1.KeycloakIdentityProvider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakIdentityProvider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(parsed.Alias),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakIdentityProvider",
		Name:       idp.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     idp,
	}, nil
}

// TransformComponent transforms a component JSON to KeycloakComponent
func (t *Transformer) TransformComponent(raw json.RawMessage) (ExportedResource, error) {
	var parsed struct {
		Name         string `json:"name"`
		ProviderType string `json:"providerType"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields and sensitive config
	definition := removeServerFields(raw, "id", "parentId")

	// Create unique name combining name and provider type
	name := parsed.Name
	if parsed.ProviderType != "" {
		// Extract short provider type name
		parts := strings.Split(parsed.ProviderType, ".")
		shortType := parts[len(parts)-1]
		name = shortType + "-" + parsed.Name
	}

	component := &keycloakv1beta1.KeycloakComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakComponent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakComponentSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakComponent",
		Name:       component.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     component,
	}, nil
}

// TransformOrganization transforms an organization JSON to KeycloakOrganization
func (t *Transformer) TransformOrganization(raw json.RawMessage) (ExportedResource, error) {
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields
	definition := removeServerFields(raw, "id")

	org := &keycloakv1beta1.KeycloakOrganization{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakOrganization",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(parsed.Name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakOrganizationSpec{
			RealmRef: &keycloakv1beta1.ResourceRef{
				Name: t.opts.RealmRef,
			},
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	return ExportedResource{
		Kind:       "KeycloakOrganization",
		Name:       org.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     org,
	}, nil
}

// TransformProtocolMapper transforms a protocol mapper JSON to KeycloakProtocolMapper
func (t *Transformer) TransformProtocolMapper(raw json.RawMessage, clientID, scopeName string) (ExportedResource, error) {
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ExportedResource{}, err
	}

	// Remove server-managed fields
	definition := removeServerFields(raw, "id")

	// Create unique name
	var name string
	var parentRef string
	if clientID != "" {
		name = clientID + "-" + parsed.Name
		parentRef = sanitizeName(clientID)
	} else {
		name = scopeName + "-" + parsed.Name
		parentRef = sanitizeName(scopeName)
	}

	mapper := &keycloakv1beta1.KeycloakProtocolMapper{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakProtocolMapper",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
			Definition: runtime.RawExtension{Raw: definition},
		},
	}

	if clientID != "" {
		mapper.Spec.ClientRef = &keycloakv1beta1.ResourceRef{
			Name: parentRef,
		}
	} else {
		mapper.Spec.ClientScopeRef = &keycloakv1beta1.ResourceRef{
			Name: parentRef,
		}
	}

	return ExportedResource{
		Kind:       "KeycloakProtocolMapper",
		Name:       mapper.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     mapper,
	}, nil
}

// TransformRoleMapping creates a KeycloakRoleMapping
func (t *Transformer) TransformRoleMapping(subjectType, subjectName, roleName, clientID, clientUUID string) (ExportedResource, error) {
	// Create unique name
	name := subjectName + "-" + roleName
	if clientID != "" {
		name = subjectName + "-" + clientID + "-" + roleName
	}

	mapping := &keycloakv1beta1.KeycloakRoleMapping{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Kind:       "KeycloakRoleMapping",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeName(name),
			Namespace: t.opts.TargetNamespace,
		},
		Spec: keycloakv1beta1.KeycloakRoleMappingSpec{},
	}

	// Set subject
	subjectRef := &keycloakv1beta1.ResourceRef{
		Name: sanitizeName(subjectName),
	}
	if subjectType == "user" {
		mapping.Spec.Subject = keycloakv1beta1.RoleMappingSubject{
			UserRef: subjectRef,
		}
	} else {
		mapping.Spec.Subject = keycloakv1beta1.RoleMappingSubject{
			GroupRef: subjectRef,
		}
	}

	// Set role reference
	roleRefName := roleName
	if clientID != "" {
		roleRefName = clientID + "-" + roleName
	}
	mapping.Spec.RoleRef = &keycloakv1beta1.ResourceRef{
		Name: sanitizeName(roleRefName),
	}

	return ExportedResource{
		Kind:       "KeycloakRoleMapping",
		Name:       mapping.Name,
		APIVersion: "keycloak.hostzero.com/v1beta1",
		Object:     mapping,
	}, nil
}

// sanitizeName converts a name to a valid Kubernetes resource name
func sanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace invalid characters with dashes
	re := regexp.MustCompile(`[^a-z0-9-]`)
	name = re.ReplaceAllString(name, "-")

	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")

	// Collapse multiple dashes
	re = regexp.MustCompile(`-+`)
	name = re.ReplaceAllString(name, "-")

	// Truncate to 63 characters (Kubernetes name limit)
	if len(name) > 63 {
		name = name[:63]
		// Remove trailing dash after truncation
		name = strings.TrimRight(name, "-")
	}

	// Ensure name is not empty
	if name == "" {
		name = "unnamed"
	}

	return name
}

// removeServerFields removes specified fields from JSON
func removeServerFields(raw json.RawMessage, fields ...string) json.RawMessage {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return raw
	}

	for _, field := range fields {
		// Handle nested fields (e.g., "config.clientSecret")
		parts := strings.Split(field, ".")
		if len(parts) == 1 {
			delete(data, field)
		} else {
			// Navigate to nested field
			current := data
			for i := 0; i < len(parts)-1; i++ {
				if nested, ok := current[parts[i]].(map[string]interface{}); ok {
					current = nested
				} else {
					current = nil
					break
				}
			}
			if current != nil {
				delete(current, parts[len(parts)-1])
			}
		}
	}

	result, err := json.Marshal(data)
	if err != nil {
		return raw
	}

	return result
}
