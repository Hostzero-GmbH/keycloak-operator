package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ClusterKeycloakRealmSpec defines the desired state of ClusterKeycloakRealm
type ClusterKeycloakRealmSpec struct {
	// ClusterInstanceRef references the ClusterKeycloakInstance
	// +kubebuilder:validation:Required
	ClusterInstanceRef ClusterResourceRef `json:"clusterInstanceRef"`

	// Definition is the realm definition in Keycloak JSON format
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// ClusterResourceRef references a cluster-scoped resource
type ClusterResourceRef struct {
	// Name is the name of the cluster resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// ClusterKeycloakRealmStatus defines the observed state of ClusterKeycloakRealm
type ClusterKeycloakRealmStatus struct {
	// Ready indicates if the realm exists in Keycloak
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the API path for this realm
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.clusterInstanceRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterKeycloakRealm is a cluster-scoped realm resource
type ClusterKeycloakRealm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterKeycloakRealmSpec   `json:"spec,omitempty"`
	Status ClusterKeycloakRealmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterKeycloakRealmList contains a list of ClusterKeycloakRealm
type ClusterKeycloakRealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterKeycloakRealm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterKeycloakRealm{}, &ClusterKeycloakRealmList{})
}
