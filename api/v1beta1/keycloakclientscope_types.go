package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakClientScopeSpec defines the desired state of KeycloakClientScope
type KeycloakClientScopeSpec struct {
	// RealmRef references the KeycloakRealm this client scope belongs to
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition is the client scope definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakClientScopeStatus defines the observed state of KeycloakClientScope
type KeycloakClientScopeStatus struct {
	// Ready indicates if the client scope exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ScopeID is the internal Keycloak client scope ID
	// +optional
	ScopeID string `json:"scopeId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakClientScope is the Schema for the keycloakclientscopes API
type KeycloakClientScope struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakClientScopeSpec   `json:"spec,omitempty"`
	Status KeycloakClientScopeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakClientScopeList contains a list of KeycloakClientScope
type KeycloakClientScopeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakClientScope `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakClientScope{}, &KeycloakClientScopeList{})
}
