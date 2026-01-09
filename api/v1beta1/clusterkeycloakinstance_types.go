package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterKeycloakInstanceSpec defines the desired state of ClusterKeycloakInstance
// It mirrors KeycloakInstanceSpec but is cluster-scoped
type ClusterKeycloakInstanceSpec struct {
	// BaseUrl is the URL of the Keycloak server (e.g., http://keycloak:8080)
	// +kubebuilder:validation:Required
	BaseUrl string `json:"baseUrl"`

	// Credentials contains the reference to the admin credentials secret
	// +kubebuilder:validation:Required
	Credentials ClusterCredentialsSpec `json:"credentials"`

	// Realm is the admin realm (defaults to "master")
	// +optional
	Realm *string `json:"realm,omitempty"`

	// Client contains optional service account client configuration
	// +optional
	Client *ClientAuthSpec `json:"client,omitempty"`

	// Token contains optional token caching configuration
	// +optional
	Token *TokenSpec `json:"token,omitempty"`
}

// ClusterCredentialsSpec defines admin credentials for cluster-scoped instances
type ClusterCredentialsSpec struct {
	// SecretRef contains the reference to the secret with credentials
	// +kubebuilder:validation:Required
	SecretRef ClusterSecretRefSpec `json:"secretRef"`
}

// ClusterSecretRefSpec defines a reference to a secret for cluster-scoped resources
type ClusterSecretRefSpec struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret (required for cluster-scoped resources)
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// UsernameKey is the key in the secret for the username (defaults to "username")
	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the secret for the password (defaults to "password")
	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ClusterKeycloakInstanceStatus defines the observed state of ClusterKeycloakInstance
type ClusterKeycloakInstanceStatus struct {
	// Ready indicates if the Keycloak instance is accessible
	Ready bool `json:"ready"`

	// Version is the Keycloak server version
	// +optional
	Version string `json:"version,omitempty"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information about the status
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the API path for this resource
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ckci,categories={keycloak,all}
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the instance is ready"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.baseUrl`,description="The base URL of the Keycloak instance"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,description="Keycloak server version"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterKeycloakInstance makes a Keycloak server known to the operator at the cluster level
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
