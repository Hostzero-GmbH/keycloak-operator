package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakInstanceSpec defines the desired state of KeycloakInstance
type KeycloakInstanceSpec struct {
	// BaseUrl is the URL of the Keycloak server (e.g., http://keycloak:8080)
	// +kubebuilder:validation:Required
	BaseUrl string `json:"baseUrl"`

	// Auth selects how the operator authenticates to Keycloak.
	// Exactly one of auth.passwordGrant or auth.clientCredentials must be set.
	// +kubebuilder:validation:Required
	Auth AuthSpec `json:"auth"`

	// Realm is the admin realm (defaults to "master")
	// +optional
	Realm *string `json:"realm,omitempty"`

	// TLS configures how the operator verifies the Keycloak server certificate.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`

	// Token contains optional token caching configuration
	// +optional
	Token *TokenSpec `json:"token,omitempty"`
}

// TLSSpec configures TLS verification for the Keycloak HTTPS endpoint.
// Setting insecureSkipVerify disables certificate validation entirely, in
// which case caCert is ignored.
type TLSSpec struct {
	// CACert references a Secret or ConfigMap holding a PEM-encoded CA bundle
	// used to verify the Keycloak server certificate.
	// +optional
	CACert *CACertSource `json:"caCert,omitempty"`

	// InsecureSkipVerify disables TLS certificate verification. Do not enable
	// in production.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// CACertSource references a Secret or ConfigMap key containing a PEM-encoded
// CA bundle. Exactly one of secretRef or configMapRef must be set.
// +kubebuilder:validation:XValidation:rule="has(self.secretRef) != has(self.configMapRef)",message="exactly one of caCert.secretRef or caCert.configMapRef must be set"
type CACertSource struct {
	// +optional
	SecretRef *CACertSecretRefSpec `json:"secretRef,omitempty"`

	// +optional
	ConfigMapRef *CACertConfigMapRefSpec `json:"configMapRef,omitempty"`
}

// CACertSecretRefSpec references a Secret key holding a PEM-encoded CA bundle.
type CACertSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace defaults to the KeycloakInstance namespace when unset.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// +kubebuilder:default="ca.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// CACertConfigMapRefSpec references a ConfigMap key holding a PEM-encoded CA
// bundle (e.g. kube-root-ca.crt or a cert-manager CA bundle).
type CACertConfigMapRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace defaults to the KeycloakInstance namespace when unset.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// +kubebuilder:default="ca.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// AuthSpec defines the authentication configuration for connecting to Keycloak.
// +kubebuilder:validation:XValidation:rule="has(self.passwordGrant) != has(self.clientCredentials)",message="exactly one of auth.passwordGrant or auth.clientCredentials must be set"
type AuthSpec struct {
	// PasswordGrant configures resource-owner password grant authentication
	// against a user account (typically the master-realm admin).
	// +optional
	PasswordGrant *PasswordGrantSpec `json:"passwordGrant,omitempty"`

	// ClientCredentials configures OAuth2 client_credentials grant
	// authentication via a confidential client / service account.
	// +optional
	ClientCredentials *ClientCredentialsSpec `json:"clientCredentials,omitempty"`
}

// PasswordGrantSpec configures password-grant authentication.
type PasswordGrantSpec struct {
	// Username, when set, overrides the value read from secretRef.usernameKey.
	// The admin username is not a secret, so providing it inline is allowed.
	// +optional
	Username *string `json:"username,omitempty"`

	// +kubebuilder:validation:Required
	SecretRef PasswordGrantSecretRefSpec `json:"secretRef"`
}

// PasswordGrantSecretRefSpec references a Secret containing admin credentials.
type PasswordGrantSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace defaults to the KeycloakInstance namespace when unset.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// UsernameKey is ignored when PasswordGrantSpec.Username is set.
	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ClientCredentialsSpec configures OAuth2 client_credentials authentication.
type ClientCredentialsSpec struct {
	// ClientID, when set, overrides the value read from secretRef.clientIdKey.
	// The client ID is not a secret, so providing it inline is allowed.
	// +optional
	ClientID *string `json:"clientId,omitempty"`

	// +kubebuilder:validation:Required
	SecretRef ClientCredentialsSecretRefSpec `json:"secretRef"`
}

// ClientCredentialsSecretRefSpec references a Secret containing client credentials.
type ClientCredentialsSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace defaults to the KeycloakInstance namespace when unset.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// ClientIdKey is ignored when ClientCredentialsSpec.ClientID is set.
	// +kubebuilder:default="client-id"
	// +optional
	ClientIdKey string `json:"clientIdKey,omitempty"`

	// +kubebuilder:default="client-secret"
	// +optional
	ClientSecretKey string `json:"clientSecretKey,omitempty"`
}

// TokenSpec defines token caching configuration
type TokenSpec struct {
	// SecretName is the name of the secret to cache the token
	// +optional
	SecretName *string `json:"secretName,omitempty"`

	// TokenKey is the key in the secret for the token
	// +optional
	TokenKey *string `json:"tokenKey,omitempty"`

	// ExpiresKey is the key in the secret for the token expiration
	// +optional
	ExpiresKey *string `json:"expiresKey,omitempty"`
}

// KeycloakInstanceStatus defines the observed state of KeycloakInstance
type KeycloakInstanceStatus struct {
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the instance is ready"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.baseUrl`,description="The base URL of the Keycloak instance"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,description="Keycloak server version"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kci,categories={keycloak,all}

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
