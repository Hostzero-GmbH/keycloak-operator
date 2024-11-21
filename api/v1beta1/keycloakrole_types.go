package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakRoleSpec defines the desired state of KeycloakRole
type KeycloakRoleSpec struct {
	// InstanceRef references the KeycloakInstance this role belongs to
	// +kubebuilder:validation:Required
	InstanceRef ResourceRef `json:"instanceRef"`

	// Definition is the role definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakRoleStatus defines the observed state of KeycloakRole
type KeycloakRoleStatus struct {
	// Ready indicates if the role exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// RoleID is the internal Keycloak role ID
	// +optional
	RoleID string `json:"roleId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakRole is the Schema for the keycloakroles API
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
