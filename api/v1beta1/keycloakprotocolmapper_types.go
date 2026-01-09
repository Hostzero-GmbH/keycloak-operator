package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakProtocolMapperSpec defines the desired state of KeycloakProtocolMapper
type KeycloakProtocolMapperSpec struct {
	// ClientRef is a reference to a KeycloakClient
	// One of clientRef or clientScopeRef must be specified
	// +optional
	ClientRef *ResourceRef `json:"clientRef,omitempty"`

	// ClientScopeRef is a reference to a KeycloakClientScope
	// One of clientRef or clientScopeRef must be specified
	// +optional
	ClientScopeRef *ResourceRef `json:"clientScopeRef,omitempty"`

	// Definition contains the Keycloak ProtocolMapperRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// ProtocolMapperDefinition represents the Keycloak ProtocolMapperRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type ProtocolMapperDefinition struct {
	// Name is the mapper name (required)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Protocol is the protocol (openid-connect or saml)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=openid-connect;saml
	Protocol string `json:"protocol"`

	// ProtocolMapper is the mapper type
	// +kubebuilder:validation:Required
	ProtocolMapper string `json:"protocolMapper"`

	// ConsentRequired indicates if consent is required
	// +optional
	ConsentRequired *bool `json:"consentRequired,omitempty"`

	// Config contains mapper configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// KeycloakProtocolMapperStatus defines the observed state of KeycloakProtocolMapper
type KeycloakProtocolMapperStatus struct {
	// Ready indicates if the protocol mapper is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this protocol mapper
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// MapperID is the Keycloak internal mapper ID
	// +optional
	MapperID string `json:"mapperID,omitempty"`

	// MapperName is the mapper name in Keycloak
	// +optional
	MapperName string `json:"mapperName,omitempty"`

	// ParentType indicates if parent is "client" or "clientScope"
	// +optional
	ParentType string `json:"parentType,omitempty"`

	// ParentID is the parent client or clientScope ID
	// +optional
	ParentID string `json:"parentID,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the protocol mapper is ready"
// +kubebuilder:printcolumn:name="Mapper",type=string,JSONPath=`.status.mapperName`,description="Mapper name"
// +kubebuilder:printcolumn:name="Parent",type=string,JSONPath=`.status.parentType`,description="Parent type (client/clientScope)"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcpm,categories={keycloak,all}

// KeycloakProtocolMapper defines a protocol mapper within a KeycloakClient or KeycloakClientScope
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

// GetClientRef returns the client reference (nil if using clientScopeRef)
func (p *KeycloakProtocolMapper) GetClientRef() *ResourceRef {
	return p.Spec.ClientRef
}

// GetClientScopeRef returns the client scope reference (nil if using clientRef)
func (p *KeycloakProtocolMapper) GetClientScopeRef() *ResourceRef {
	return p.Spec.ClientScopeRef
}

// IsClientMapper returns true if this mapper belongs to a client
func (p *KeycloakProtocolMapper) IsClientMapper() bool {
	return p.Spec.ClientRef != nil
}

// IsClientScopeMapper returns true if this mapper belongs to a client scope
func (p *KeycloakProtocolMapper) IsClientScopeMapper() bool {
	return p.Spec.ClientScopeRef != nil
}
