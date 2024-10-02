package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakClientSpec defines the desired state of KeycloakClient
type KeycloakClientSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition contains the Keycloak ClientRepresentation
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Required
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakClientStatus defines the observed state of KeycloakClient
type KeycloakClientStatus struct {
	// Ready indicates if the client is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ClientUUID is the Keycloak internal ID
	// +optional
	ClientUUID string `json:"clientUUID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the client is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakClient defines a client within a KeycloakRealm
type KeycloakClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakClientSpec   `json:"spec,omitempty"`
	Status KeycloakClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakClientList contains a list of KeycloakClient
type KeycloakClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakClient{}, &KeycloakClientList{})
}
