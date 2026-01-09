package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakOrganizationSpec defines the desired state of KeycloakOrganization
type KeycloakOrganizationSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// Definition contains the Keycloak OrganizationRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// OrganizationDefinition represents the Keycloak OrganizationRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type OrganizationDefinition struct {
	// Name is the organization name (required)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Alias is the URL-friendly identifier for the organization
	// +optional
	Alias string `json:"alias,omitempty"`

	// Description of the organization
	// +optional
	Description string `json:"description,omitempty"`

	// Enabled indicates whether the organization is enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Domains associated with the organization
	// +optional
	Domains []OrganizationDomain `json:"domains,omitempty"`

	// Attributes for custom organization attributes
	// +optional
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// OrganizationDomain represents a domain associated with an organization
type OrganizationDomain struct {
	// Name is the domain name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Verified indicates if the domain is verified
	// +optional
	Verified bool `json:"verified,omitempty"`
}

// KeycloakOrganizationStatus defines the observed state of KeycloakOrganization
type KeycloakOrganizationStatus struct {
	// Ready indicates if the organization is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this organization
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// OrganizationID is the Keycloak internal organization ID
	// +optional
	OrganizationID string `json:"organizationID,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the organization is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcorg,categories={keycloak,all}

// KeycloakOrganization defines an organization within a KeycloakRealm
// NOTE: Organizations require Keycloak 26.0.0 or later
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

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (o *KeycloakOrganization) GetRealmRef() *ResourceRef {
	return o.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (o *KeycloakOrganization) GetClusterRealmRef() *ClusterResourceRef {
	return o.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this organization references a ClusterKeycloakRealm
func (o *KeycloakOrganization) UsesClusterRealm() bool {
	return o.Spec.ClusterRealmRef != nil
}
