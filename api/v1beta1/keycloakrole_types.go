package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakRoleSpec defines the desired state of KeycloakRole
type KeycloakRoleSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ClientRef is a reference to a KeycloakClient for client-level roles
	// If not specified, the role is a realm-level role
	// +optional
	ClientRef *ResourceRef `json:"clientRef,omitempty"`

	// Definition contains the Keycloak RoleRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// RoleRepresentation represents the Keycloak RoleRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type RoleRepresentation struct {
	// Name is the role name (required)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Description of the role
	// +optional
	Description string `json:"description,omitempty"`

	// Composite indicates if this is a composite role
	// +optional
	Composite *bool `json:"composite,omitempty"`

	// ClientRole indicates if this is a client role
	// +optional
	ClientRole *bool `json:"clientRole,omitempty"`

	// ContainerId is the container ID (realm or client ID)
	// +optional
	ContainerId string `json:"containerId,omitempty"`

	// Attributes for custom role attributes
	// +optional
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// KeycloakRoleStatus defines the observed state of KeycloakRole
type KeycloakRoleStatus struct {
	// Ready indicates if the role is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this role
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// RoleID is the Keycloak internal role ID
	// +optional
	RoleID string `json:"roleID,omitempty"`

	// RoleName is the role name in Keycloak
	// +optional
	RoleName string `json:"roleName,omitempty"`

	// IsClientRole indicates if this is a client role
	// +optional
	IsClientRole bool `json:"isClientRole,omitempty"`

	// ClientID is the client ID if this is a client role
	// +optional
	ClientID string `json:"clientID,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the role is ready"
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.status.roleName`,description="Role name"
// +kubebuilder:printcolumn:name="Client",type=string,JSONPath=`.status.clientID`,description="Client ID (for client roles)"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcr,categories={keycloak,all}

// KeycloakRole defines a role within a KeycloakRealm or KeycloakClient
type KeycloakRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakRoleSpec   `json:"spec,omitempty"`
	Status KeycloakRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRoleList contains a list of KeycloakRole
type KeycloakRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakRole{}, &KeycloakRoleList{})
}

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (r *KeycloakRole) GetRealmRef() *ResourceRef {
	return r.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (r *KeycloakRole) GetClusterRealmRef() *ClusterResourceRef {
	return r.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this role references a ClusterKeycloakRealm
func (r *KeycloakRole) UsesClusterRealm() bool {
	return r.Spec.ClusterRealmRef != nil
}

// IsClientRole returns true if this is a client-level role
func (r *KeycloakRole) IsClientRole() bool {
	return r.Spec.ClientRef != nil
}
