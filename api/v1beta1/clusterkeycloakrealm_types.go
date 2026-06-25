package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ClusterKeycloakRealmSpec defines the desired state of ClusterKeycloakRealm
// +kubebuilder:validation:XValidation:rule="has(self.instanceRef) != has(self.clusterInstanceRef)",message="exactly one of instanceRef or clusterInstanceRef must be set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.realmName) || self.realmName == oldSelf.realmName",message="spec.realmName is immutable once set"
type ClusterKeycloakRealmSpec struct {
	// InstanceRef is a reference to a namespaced KeycloakInstance
	// One of instanceRef or clusterInstanceRef must be specified
	// +optional
	InstanceRef *NamespacedRef `json:"instanceRef,omitempty"`

	// ClusterInstanceRef is a reference to a ClusterKeycloakInstance
	// One of instanceRef or clusterInstanceRef must be specified
	// +optional
	ClusterInstanceRef *ClusterResourceRef `json:"clusterInstanceRef,omitempty"`

	// RealmName is the name of the realm in Keycloak. It is immutable once set:
	// renaming a realm in Keycloak is destructive and would orphan it.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RealmName *string `json:"realmName,omitempty"`

	// SmtpSecretRef is a reference to a Kubernetes Secret containing SMTP credentials.
	// When set, the secret values are injected into definition.smtpServer.user and
	// definition.smtpServer.password before syncing to Keycloak.
	// +optional
	SmtpSecretRef *ClusterSmtpSecretRefSpec `json:"smtpSecretRef,omitempty"`

	// Definition contains the Keycloak RealmRepresentation. Set the realm name
	// via spec.realmName.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// NamespacedRef is a reference to a namespaced resource (required namespace)
type NamespacedRef struct {
	// Name of the resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the resource
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// ClusterKeycloakRealmStatus defines the observed state of ClusterKeycloakRealm
type ClusterKeycloakRealmStatus struct {
	// Ready indicates if the realm is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this realm
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// RealmName is the actual realm name in Keycloak
	// +optional
	RealmName string `json:"realmName,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ckcrm,categories={keycloak,all}
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the realm is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Realm",type=string,JSONPath=`.status.realmName`,description="Realm name in Keycloak"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterKeycloakRealm defines a realm within a KeycloakInstance at the cluster level
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

// UsesClusterInstance returns true if this realm references a ClusterKeycloakInstance
func (r *ClusterKeycloakRealm) UsesClusterInstance() bool {
	return r.Spec.ClusterInstanceRef != nil
}
