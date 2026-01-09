package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakGroupSpec defines the desired state of KeycloakGroup
type KeycloakGroupSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ParentGroupRef is a reference to a parent KeycloakGroup (for nested groups)
	// +optional
	ParentGroupRef *ResourceRef `json:"parentGroupRef,omitempty"`

	// Definition contains the Keycloak GroupRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// KeycloakGroupStatus defines the observed state of KeycloakGroup
type KeycloakGroupStatus struct {
	// Ready indicates if the group is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this group
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// GroupID is the Keycloak internal group ID
	// +optional
	GroupID string `json:"groupID,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the group is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcg,categories={keycloak,all}

// KeycloakGroup defines a group within a KeycloakRealm
type KeycloakGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakGroupSpec   `json:"spec,omitempty"`
	Status KeycloakGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakGroupList contains a list of KeycloakGroup
type KeycloakGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakGroup{}, &KeycloakGroupList{})
}
