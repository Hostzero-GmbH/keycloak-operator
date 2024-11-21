package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakGroupSpec defines the desired state of KeycloakGroup
type KeycloakGroupSpec struct {
	// RealmRef references the KeycloakRealm this group belongs to
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition is the group definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakGroupStatus defines the observed state of KeycloakGroup
type KeycloakGroupStatus struct {
	// Ready indicates if the group exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// GroupID is the internal Keycloak group ID
	// +optional
	GroupID string `json:"groupId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakGroup is the Schema for the keycloakgroups API
type KeycloakGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakGroupSpec   `json:"spec,omitempty"`
	Status KeycloakGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakGroupList contains a list of KeycloakGroup
type KeycloakGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakGroup{}, &KeycloakGroupList{})
}
