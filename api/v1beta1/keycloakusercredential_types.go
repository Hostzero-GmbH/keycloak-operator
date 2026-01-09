package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakUserCredentialSpec defines the desired state of KeycloakUserCredential
type KeycloakUserCredentialSpec struct {
	// UserRef is a reference to a KeycloakUser
	// +kubebuilder:validation:Required
	UserRef ResourceRef `json:"userRef"`

	// UserSecret defines the secret containing the credentials
	// +kubebuilder:validation:Required
	UserSecret CredentialSecretSpec `json:"userSecret"`
}

// CredentialSecretSpec defines the secret containing user credentials
type CredentialSecretSpec struct {
	// SecretName is the name of the Kubernetes secret
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Create indicates whether to create the secret if it doesn't exist
	// +optional
	Create bool `json:"create,omitempty"`

	// UsernameKey is the key for the username in the secret
	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key for the password in the secret
	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`

	// EmailKey is the key for the email in the secret
	// +optional
	EmailKey string `json:"emailKey,omitempty"`

	// PasswordPolicy configures password generation
	// +optional
	PasswordPolicy *PasswordPolicySpec `json:"passwordPolicy,omitempty"`
}

// PasswordPolicySpec defines password generation policy
type PasswordPolicySpec struct {
	// Length is the password length (default 24)
	// +kubebuilder:default=24
	// +optional
	Length int `json:"length,omitempty"`

	// IncludeNumbers includes numbers in the password
	// +kubebuilder:default=true
	// +optional
	IncludeNumbers *bool `json:"includeNumbers,omitempty"`

	// IncludeSymbols includes symbols in the password
	// +kubebuilder:default=true
	// +optional
	IncludeSymbols *bool `json:"includeSymbols,omitempty"`
}

// KeycloakUserCredentialStatus defines the observed state of KeycloakUserCredential
type KeycloakUserCredentialStatus struct {
	// Ready indicates if the credentials are synchronized
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for the user
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// SecretCreated indicates if the secret was created by the operator
	// +optional
	SecretCreated bool `json:"secretCreated,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// PasswordHash is a hash of the last synchronized password (for change detection)
	// +optional
	PasswordHash string `json:"passwordHash,omitempty"`

	// SecretResourceVersion is the resource version of the secret when last synced
	// +optional
	SecretResourceVersion string `json:"secretResourceVersion,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the credentials are synchronized"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcuc,categories={keycloak,all}

// KeycloakUserCredential manages credentials for a KeycloakUser
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

// GetUserRef returns the user reference
func (c *KeycloakUserCredential) GetUserRef() ResourceRef {
	return c.Spec.UserRef
}
