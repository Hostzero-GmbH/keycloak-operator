package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakIdentityProviderMapperSpec defines the desired state of KeycloakIdentityProviderMapper
type KeycloakIdentityProviderMapperSpec struct {
	// IdentityProviderRef is a reference to a KeycloakIdentityProvider that owns
	// this mapper. The realm and Keycloak instance are derived from the parent
	// identity provider.
	// +kubebuilder:validation:Required
	IdentityProviderRef ResourceRef `json:"identityProviderRef"`

	// Name is the mapper name in Keycloak (defaults to metadata.name).
	// This is the recommended way to set the mapper name and takes precedence
	// over any name supplied inside spec.definition.
	// +optional
	Name *string `json:"name,omitempty"`

	// Definition contains the Keycloak IdentityProviderMapperRepresentation.
	// The identityProviderAlias field is auto-injected from the parent
	// KeycloakIdentityProvider at reconcile time and does not need to be set
	// here.
	// Deprecated: setting the identifier (name) inside definition is deprecated;
	// use the first-class spec.name field instead. A name inside definition is
	// still honored in this release but will be rejected in a future release.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakIdentityProviderMapperStatus defines the observed state of KeycloakIdentityProviderMapper
type KeycloakIdentityProviderMapperStatus struct {
	// Ready indicates if the identity provider mapper is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this identity provider mapper
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// MapperID is the Keycloak internal mapper ID
	// +optional
	MapperID string `json:"mapperID,omitempty"`

	// MapperName is the mapper name in Keycloak
	// +optional
	MapperName string `json:"mapperName,omitempty"`

	// IdentityProviderAlias is the alias of the parent identity provider
	// +optional
	IdentityProviderAlias string `json:"identityProviderAlias,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the identity provider mapper is ready"
// +kubebuilder:printcolumn:name="Mapper",type=string,JSONPath=`.status.mapperName`,description="Mapper name"
// +kubebuilder:printcolumn:name="IdP",type=string,JSONPath=`.status.identityProviderAlias`,description="Parent identity provider alias"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcidpm,categories={keycloak,all}

// KeycloakIdentityProviderMapper defines a mapper attached to a KeycloakIdentityProvider
type KeycloakIdentityProviderMapper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakIdentityProviderMapperSpec   `json:"spec,omitempty"`
	Status KeycloakIdentityProviderMapperStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakIdentityProviderMapperList contains a list of KeycloakIdentityProviderMapper
type KeycloakIdentityProviderMapperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakIdentityProviderMapper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakIdentityProviderMapper{}, &KeycloakIdentityProviderMapperList{})
}
