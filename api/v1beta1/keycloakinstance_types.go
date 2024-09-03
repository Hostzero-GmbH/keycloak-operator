package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakInstanceSpec defines the desired state of KeycloakInstance
type KeycloakInstanceSpec struct {
	// BaseUrl is the URL of the Keycloak server (e.g., http://keycloak:8080)
	// +kubebuilder:validation:Required
	BaseUrl string `json:"baseUrl"`

	// Credentials contains the reference to the admin credentials secret
	// +kubebuilder:validation:Required
	Credentials CredentialsSpec `json:"credentials"`

	// Realm is the admin realm (defaults to "master")
	// +optional
	Realm *string `json:"realm,omitempty"`
}

// CredentialsSpec defines admin credentials configuration
type CredentialsSpec struct {
	// SecretRef contains the reference to the secret with credentials
	// +kubebuilder:validation:Required
	SecretRef SecretRefSpec `json:"secretRef"`
}

// SecretRefSpec defines a reference to a secret
type SecretRefSpec struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret (defaults to resource namespace)
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// UsernameKey is the key in the secret for the username (defaults to "username")
	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the secret for the password (defaults to "password")
	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// KeycloakInstanceStatus defines the observed state of KeycloakInstance
type KeycloakInstanceStatus struct {
	// Ready indicates if the Keycloak instance is accessible
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information about the status
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the instance is ready"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.baseUrl`,description="The base URL of the Keycloak instance"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakInstance makes a Keycloak server known to the operator
type KeycloakInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakInstanceSpec   `json:"spec,omitempty"`
	Status KeycloakInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakInstanceList contains a list of KeycloakInstance
type KeycloakInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakInstance{}, &KeycloakInstanceList{})
}
