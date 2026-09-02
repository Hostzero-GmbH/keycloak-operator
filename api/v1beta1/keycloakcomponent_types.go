package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConfigSecretRefMapping maps a single key from a Kubernetes Secret to a
// specific config key in the component's definition.config. Unlike
// ConfigSecretRef (which merges all secret keys by name), this allows explicit
// source-to-target mapping — needed when cert-manager key names (tls.key,
// tls.crt) don't match Keycloak config names (privateKey, certificate).
type ConfigSecretRefMapping struct {
	// SecretName is the name of the Kubernetes Secret in the same namespace
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Key is the key within the Secret that holds the value
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// ConfigKey is the target key in definition.config where the value is
	// injected as a single-element string list (component config is map[string][]string)
	// +kubebuilder:validation:Required
	ConfigKey string `json:"configKey"`
}

// KeycloakComponentSpec defines the desired state of KeycloakComponent
// +kubebuilder:validation:XValidation:rule="has(self.realmRef) != has(self.clusterRealmRef)",message="exactly one of realmRef or clusterRealmRef must be set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.name) || self.name == oldSelf.name",message="spec.name is immutable once set"
// +kubebuilder:validation:XValidation:rule="!(has(self.configSecretRef) && has(self.configSecretRefs))",message="configSecretRef and configSecretRefs are mutually exclusive"
type KeycloakComponentSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// Name is the component name in Keycloak. Immutable once set. The
	// providerType is set in spec.definition.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name *string `json:"name,omitempty"`

	// ConfigSecretRef is a reference to a Kubernetes Secret whose data entries
	// are merged into definition.config before syncing to Keycloak. Each secret
	// value is wrapped as a single-element list to match ComponentRepresentation
	// config (map[string][]string). Secret values take precedence over values
	// specified inline in definition.config.
	// +optional
	ConfigSecretRef *ConfigSecretRef `json:"configSecretRef,omitempty"`

	// ConfigSecretRefs maps individual Secret keys to specific config keys in
	// definition.config. Use this instead of configSecretRef when Secret key
	// names don't match Keycloak config key names (e.g. cert-manager
	// tls.key → privateKey). Unlike configSecretRef, a config key set both
	// inline in definition.config and via a mapping is rejected rather than
	// overridden, to surface the ambiguity. Mutually exclusive with
	// configSecretRef.
	// +optional
	ConfigSecretRefs []ConfigSecretRefMapping `json:"configSecretRefs,omitempty"`

	// Definition contains the Keycloak ComponentRepresentation. Set the component
	// name via spec.name.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
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

	// LastAppliedDefinitionHash is a hash of the last successfully applied
	// definition (after secret merging). Keycloak masks secret config values
	// on read, so this is the only way to detect that the desired secret
	// changed and must be pushed.
	// +optional
	LastAppliedDefinitionHash string `json:"lastAppliedDefinitionHash,omitempty"`

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
