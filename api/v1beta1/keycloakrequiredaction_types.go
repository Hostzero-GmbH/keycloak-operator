package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakRequiredActionSpec defines the desired state of KeycloakRequiredAction
type KeycloakRequiredActionSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// Definition contains the Keycloak RequiredActionProviderRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakRequiredActionStatus defines the observed state of KeycloakRequiredAction
type KeycloakRequiredActionStatus struct {
	// Ready indicates if the required action is synchronized
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this required action
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// Alias is the required action alias in Keycloak
	// +optional
	Alias string `json:"alias,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the required action is ready"
// +kubebuilder:printcolumn:name="Alias",type=string,JSONPath=`.status.alias`,description="Required action alias"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcra,categories={keycloak,all}

// KeycloakRequiredAction manages a required action provider within a Keycloak realm
type KeycloakRequiredAction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakRequiredActionSpec   `json:"spec,omitempty"`
	Status KeycloakRequiredActionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRequiredActionList contains a list of KeycloakRequiredAction
type KeycloakRequiredActionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRequiredAction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakRequiredAction{}, &KeycloakRequiredActionList{})
}

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (r *KeycloakRequiredAction) GetRealmRef() *ResourceRef {
	return r.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (r *KeycloakRequiredAction) GetClusterRealmRef() *ClusterResourceRef {
	return r.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this required action references a ClusterKeycloakRealm
func (r *KeycloakRequiredAction) UsesClusterRealm() bool {
	return r.Spec.ClusterRealmRef != nil
}
