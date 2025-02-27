package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakProtocolMapperSpec defines the desired state of KeycloakProtocolMapper
type KeycloakProtocolMapperSpec struct {
	// ClientRef references the KeycloakClient this mapper belongs to
	// +optional
	ClientRef *ResourceRef `json:"clientRef,omitempty"`

	// ClientScopeRef references the KeycloakClientScope this mapper belongs to
	// +optional
	ClientScopeRef *ResourceRef `json:"clientScopeRef,omitempty"`

	// Definition is the protocol mapper definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakProtocolMapperStatus defines the observed state of KeycloakProtocolMapper
type KeycloakProtocolMapperStatus struct {
	// Ready indicates if the mapper exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// MapperID is the internal Keycloak mapper ID
	// +optional
	MapperID string `json:"mapperId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakProtocolMapper is the Schema for the keycloakprotocolmappers API
type KeycloakProtocolMapper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakProtocolMapperSpec   `json:"spec,omitempty"`
	Status KeycloakProtocolMapperStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakProtocolMapperList contains a list of KeycloakProtocolMapper
type KeycloakProtocolMapperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakProtocolMapper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakProtocolMapper{}, &KeycloakProtocolMapperList{})
}
