package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakUserSpec defines the desired state of KeycloakUser
type KeycloakUserSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition contains the Keycloak UserRepresentation
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Required
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakUserStatus defines the observed state of KeycloakUser
type KeycloakUserStatus struct {
	// Ready indicates if the user is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// UserID is the Keycloak internal user ID
	// +optional
	UserID string `json:"userID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the user is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakUser defines a user within a KeycloakRealm
type KeycloakUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakUserSpec   `json:"spec,omitempty"`
	Status KeycloakUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakUserList contains a list of KeycloakUser
type KeycloakUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakUser{}, &KeycloakUserList{})
}
