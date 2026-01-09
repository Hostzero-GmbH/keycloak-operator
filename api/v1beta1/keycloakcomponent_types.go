package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakComponentSpec defines the desired state of KeycloakComponent
type KeycloakComponentSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// Definition contains the Keycloak ComponentRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// ComponentDefinition represents the Keycloak ComponentRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type ComponentDefinition struct {
	// Name is the component name (required)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ProviderID is the provider ID (required)
	// +kubebuilder:validation:Required
	ProviderID string `json:"providerId"`

	// ProviderType is the provider type (required)
	// Examples: org.keycloak.storage.UserStorageProvider, org.keycloak.keys.KeyProvider
	// +kubebuilder:validation:Required
	ProviderType string `json:"providerType"`

	// ParentID is the parent component ID (usually the realm ID)
	// +optional
	ParentId string `json:"parentId,omitempty"`

	// SubType is an optional subtype
	// +optional
	SubType string `json:"subType,omitempty"`

	// Config contains component configuration
	// +optional
	Config map[string][]string `json:"config,omitempty"`
}

// KeycloakComponentStatus defines the observed state of KeycloakComponent
type KeycloakComponentStatus struct {
	// Ready indicates if the component is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this component
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// ComponentID is the Keycloak internal component ID
	// +optional
	ComponentID string `json:"componentID,omitempty"`

	// ComponentName is the component name in Keycloak
	// +optional
	ComponentName string `json:"componentName,omitempty"`

	// ProviderType is the component provider type
	// +optional
	ProviderType string `json:"providerType,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the component is ready"
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.status.componentName`,description="Component name"
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.providerType`,description="Provider type"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcco,categories={keycloak,all}

// KeycloakComponent defines a component within a KeycloakRealm
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

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (c *KeycloakComponent) GetRealmRef() *ResourceRef {
	return c.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (c *KeycloakComponent) GetClusterRealmRef() *ClusterResourceRef {
	return c.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this component references a ClusterKeycloakRealm
func (c *KeycloakComponent) UsesClusterRealm() bool {
	return c.Spec.ClusterRealmRef != nil
}
