package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakUserCredentialSpec defines the desired state of KeycloakUserCredential
type KeycloakUserCredentialSpec struct {
	// UserRef references the KeycloakUser to set credentials for
	// +kubebuilder:validation:Required
	UserRef ResourceRef `json:"userRef"`

	// SecretRef references the secret containing the password
	// +kubebuilder:validation:Required
	SecretRef CredentialSecretRef `json:"secretRef"`

	// Temporary indicates if the password should be temporary
	// +optional
	Temporary bool `json:"temporary,omitempty"`
}

// CredentialSecretRef references a secret containing credentials
type CredentialSecretRef struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// PasswordKey is the key in the secret for the password
	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// KeycloakUserCredentialStatus defines the observed state of KeycloakUserCredential
type KeycloakUserCredentialStatus struct {
	// Ready indicates if the credential was set
	Ready bool `json:"ready"`

	// Status is a human-readable status
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdated is the timestamp of the last credential update
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KeycloakUserCredential is the Schema for the keycloakusercredentials API
type KeycloakUserCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakUserCredentialSpec   `json:"spec,omitempty"`
	Status KeycloakUserCredentialStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakUserCredentialList contains a list of KeycloakUserCredential
type KeycloakUserCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakUserCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakUserCredential{}, &KeycloakUserCredentialList{})
}
