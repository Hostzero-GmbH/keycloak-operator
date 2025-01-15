package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakRoleMappingSpec defines the desired state of KeycloakRoleMapping
type KeycloakRoleMappingSpec struct {
	// UserRef references the KeycloakUser to assign roles to
	// +kubebuilder:validation:Required
	UserRef ResourceRef `json:"userRef"`

	// RealmRoles is a list of realm role names to assign
	// +optional
	RealmRoles []string `json:"realmRoles,omitempty"`

	// ClientRoles maps client names to lists of role names
	// +optional
	ClientRoles map[string][]string `json:"clientRoles,omitempty"`
}

// KeycloakRoleMappingStatus defines the observed state of KeycloakRoleMapping
type KeycloakRoleMappingStatus struct {
	// Ready indicates if the role mappings are applied
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakRoleMapping is the Schema for the keycloakrolemappings API
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
