package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakOrganizationSpec defines the desired state of KeycloakOrganization
// Organizations are available in Keycloak 26+
type KeycloakOrganizationSpec struct {
	// RealmRef references the KeycloakRealm this organization belongs to
	// +kubebuilder:validation:Required
	RealmRef ResourceRef `json:"realmRef"`

	// Definition is the organization definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakOrganizationStatus defines the observed state of KeycloakOrganization
type KeycloakOrganizationStatus struct {
	// Ready indicates if the organization exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// OrganizationID is the internal Keycloak organization ID
	// +optional
	OrganizationID string `json:"organizationId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakOrganization is the Schema for the keycloakorganizations API
// Requires Keycloak 26 or later
type KeycloakOrganization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakOrganizationSpec   `json:"spec,omitempty"`
	Status KeycloakOrganizationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakOrganizationList contains a list of KeycloakOrganization
type KeycloakOrganizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakOrganization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakOrganization{}, &KeycloakOrganizationList{})
}
