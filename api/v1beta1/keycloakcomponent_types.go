package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakComponentSpec defines the desired state of KeycloakComponent
type KeycloakComponentSpec struct {
	// RealmRef references the KeycloakRealm this component belongs to
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition is the component definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakComponentStatus defines the observed state of KeycloakComponent
type KeycloakComponentStatus struct {
	// Ready indicates if the component exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ComponentID is the internal Keycloak component ID
	// +optional
	ComponentID string `json:"componentId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakComponent is the Schema for the keycloakcomponents API
type KeycloakComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakComponentSpec   `json:"spec,omitempty"`
	Status KeycloakComponentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakComponentList contains a list of KeycloakComponent
type KeycloakComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakComponent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakComponent{}, &KeycloakComponentList{})
}
