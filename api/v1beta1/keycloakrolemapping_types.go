package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakRoleMappingSpec defines the desired state of KeycloakRoleMapping
type KeycloakRoleMappingSpec struct {
	// Subject defines who the role is assigned to (user or group)
	// +kubebuilder:validation:Required
	Subject RoleMappingSubject `json:"subject"`

	// Role defines the role to assign (inline definition)
	// Either Role or RoleRef must be specified
	// +optional
	Role *RoleDefinition `json:"role,omitempty"`

	// RoleRef references an existing KeycloakRole resource
	// Either Role or RoleRef must be specified
	// +optional
	RoleRef *ResourceRef `json:"roleRef,omitempty"`
}

// RoleMappingSubject defines the target of the role mapping
type RoleMappingSubject struct {
	// UserRef references a KeycloakUser
	// +optional
	UserRef *ResourceRef `json:"userRef,omitempty"`

	// GroupRef references a KeycloakGroup
	// +optional
	GroupRef *ResourceRef `json:"groupRef,omitempty"`
}

// RoleDefinition defines a role inline
type RoleDefinition struct {
	// Name is the role name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ClientRef references a KeycloakClient for client-level roles
	// If not specified, the role is a realm-level role
	// +optional
	ClientRef *ResourceRef `json:"clientRef,omitempty"`

	// ClientID is the client ID for client-level roles (alternative to ClientRef)
	// +optional
	ClientID *string `json:"clientId,omitempty"`
}

// KeycloakRoleMappingStatus defines the observed state of KeycloakRoleMapping
type KeycloakRoleMappingStatus struct {
	// Ready indicates if the role mapping is applied
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this role mapping
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// SubjectType is either "user" or "group"
	// +optional
	SubjectType string `json:"subjectType,omitempty"`

	// SubjectID is the Keycloak ID of the subject
	// +optional
	SubjectID string `json:"subjectID,omitempty"`

	// RoleName is the resolved role name
	// +optional
	RoleName string `json:"roleName,omitempty"`

	// RoleType is either "realm" or "client"
	// +optional
	RoleType string `json:"roleType,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the role mapping is applied"
// +kubebuilder:printcolumn:name="Subject",type=string,JSONPath=`.status.subjectType`,description="Subject type (user/group)"
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.status.roleName`,description="Role name"
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.roleType`,description="Role type (realm/client)"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcrmap,categories={keycloak,all}

// KeycloakRoleMapping maps a role to a user or group
type KeycloakRoleMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakRoleMappingSpec   `json:"spec,omitempty"`
	Status KeycloakRoleMappingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRoleMappingList contains a list of KeycloakRoleMapping
type KeycloakRoleMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRoleMapping `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakRoleMapping{}, &KeycloakRoleMappingList{})
}

// GetSubject returns the subject reference (user or group)
func (r *KeycloakRoleMapping) GetSubject() RoleMappingSubject {
	return r.Spec.Subject
}

// IsUserMapping returns true if this maps to a user
func (r *KeycloakRoleMapping) IsUserMapping() bool {
	return r.Spec.Subject.UserRef != nil
}

// IsGroupMapping returns true if this maps to a group
func (r *KeycloakRoleMapping) IsGroupMapping() bool {
	return r.Spec.Subject.GroupRef != nil
}

// IsClientRole returns true if this is a client-level role
func (r *KeycloakRoleMapping) IsClientRole() bool {
	if r.Spec.Role != nil {
		return r.Spec.Role.ClientRef != nil || r.Spec.Role.ClientID != nil
	}
	return false
}
