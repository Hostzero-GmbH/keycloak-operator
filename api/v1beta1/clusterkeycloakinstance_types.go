package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterKeycloakInstanceSpec defines the desired state of ClusterKeycloakInstance
type ClusterKeycloakInstanceSpec struct {
	// BaseUrl is the URL of the Keycloak server
	// +kubebuilder:validation:Required
	BaseUrl string `json:"baseUrl"`

	// Credentials contains the reference to the admin credentials secret
	// +kubebuilder:validation:Required
	Credentials ClusterCredentialsSpec `json:"credentials"`

	// Realm is the admin realm (defaults to "master")
	// +optional
	Realm *string `json:"realm,omitempty"`
}

// ClusterCredentialsSpec defines admin credentials for cluster-scoped instance
type ClusterCredentialsSpec struct {
	// SecretRef contains the reference to the secret with credentials
	// +kubebuilder:validation:Required
	SecretRef ClusterSecretRefSpec `json:"secretRef"`
}

// ClusterSecretRefSpec defines a reference to a secret for cluster resources
type ClusterSecretRefSpec struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret (required for cluster-scoped)
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// UsernameKey is the key in the secret for the username
	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the secret for the password
	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ClusterKeycloakInstanceStatus defines the observed state of ClusterKeycloakInstance
type ClusterKeycloakInstanceStatus struct {
	// Ready indicates if the Keycloak instance is accessible
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information about the status
	// +optional
	Message string `json:"message,omitempty"`

	// Version is the Keycloak server version
	// +optional
	Version string `json:"version,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.baseUrl`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterKeycloakInstance is a cluster-scoped Keycloak instance resource
type ClusterKeycloakInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterKeycloakInstanceSpec   `json:"spec,omitempty"`
	Status ClusterKeycloakInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterKeycloakInstanceList contains a list of ClusterKeycloakInstance
type ClusterKeycloakInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterKeycloakInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterKeycloakInstance{}, &ClusterKeycloakInstanceList{})
}
