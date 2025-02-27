package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakIdentityProviderSpec defines the desired state of KeycloakIdentityProvider
type KeycloakIdentityProviderSpec struct {
	// RealmRef references the KeycloakRealm this IdP belongs to
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition is the identity provider definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakIdentityProviderStatus defines the observed state of KeycloakIdentityProvider
type KeycloakIdentityProviderStatus struct {
	// Ready indicates if the IdP exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// Alias is the IdP alias in Keycloak
	// +optional
	Alias string `json:"alias,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakIdentityProvider is the Schema for the keycloakidentityproviders API
type KeycloakIdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakIdentityProviderSpec   `json:"spec,omitempty"`
	Status KeycloakIdentityProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakIdentityProviderList contains a list of KeycloakIdentityProvider
type KeycloakIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakIdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakIdentityProvider{}, &KeycloakIdentityProviderList{})
}
