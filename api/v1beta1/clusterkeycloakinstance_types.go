package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterKeycloakInstanceSpec defines the desired state of ClusterKeycloakInstance.
// It mirrors KeycloakInstanceSpec but is cluster-scoped: secret references must
// specify a namespace explicitly.
type ClusterKeycloakInstanceSpec struct {
	// BaseUrl is the URL of the Keycloak server (e.g., http://keycloak:8080)
	// +kubebuilder:validation:Required
	BaseUrl string `json:"baseUrl"`

	// Auth selects how the operator authenticates to Keycloak.
	// Exactly one of auth.passwordGrant or auth.clientCredentials must be set.
	// +kubebuilder:validation:Required
	Auth ClusterAuthSpec `json:"auth"`

	// Realm is the admin realm (defaults to "master")
	// +optional
	Realm *string `json:"realm,omitempty"`

	// TLS configures how the operator verifies the Keycloak server certificate.
	// +optional
	TLS *ClusterTLSSpec `json:"tls,omitempty"`

	// Token contains optional token caching configuration
	// +optional
	Token *TokenSpec `json:"token,omitempty"`
}

// ClusterTLSSpec is the cluster-scoped equivalent of TLSSpec; namespace is
// required on all references.
type ClusterTLSSpec struct {
	// +optional
	CACert *ClusterCACertSource `json:"caCert,omitempty"`

	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// ClusterCACertSource references a Secret or ConfigMap key containing a
// PEM-encoded CA bundle. Exactly one of secretRef or configMapRef must be set.
// +kubebuilder:validation:XValidation:rule="has(self.secretRef) != has(self.configMapRef)",message="exactly one of caCert.secretRef or caCert.configMapRef must be set"
type ClusterCACertSource struct {
	// +optional
	SecretRef *ClusterCACertSecretRefSpec `json:"secretRef,omitempty"`

	// +optional
	ConfigMapRef *ClusterCACertConfigMapRefSpec `json:"configMapRef,omitempty"`
}

// ClusterCACertSecretRefSpec is the cluster-scoped variant; namespace required.
type ClusterCACertSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +kubebuilder:default="ca.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// ClusterCACertConfigMapRefSpec is the cluster-scoped variant; namespace required.
type ClusterCACertConfigMapRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +kubebuilder:default="ca.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// ClusterAuthSpec is the cluster-scoped equivalent of AuthSpec.
// +kubebuilder:validation:XValidation:rule="has(self.passwordGrant) != has(self.clientCredentials)",message="exactly one of auth.passwordGrant or auth.clientCredentials must be set"
type ClusterAuthSpec struct {
	// +optional
	PasswordGrant *ClusterPasswordGrantSpec `json:"passwordGrant,omitempty"`

	// +optional
	ClientCredentials *ClusterClientCredentialsSpec `json:"clientCredentials,omitempty"`
}

// ClusterPasswordGrantSpec configures password-grant authentication
// for cluster-scoped instances.
type ClusterPasswordGrantSpec struct {
	// Username, when set, overrides secretRef.usernameKey.
	// +optional
	Username *string `json:"username,omitempty"`

	// +kubebuilder:validation:Required
	SecretRef ClusterPasswordGrantSecretRefSpec `json:"secretRef"`
}

// ClusterPasswordGrantSecretRefSpec references an admin-credentials Secret.
// Namespace is required because the resource is cluster-scoped.
type ClusterPasswordGrantSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +kubebuilder:default="username"
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// +kubebuilder:default="password"
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// ClusterClientCredentialsSpec configures OAuth2 client_credentials
// authentication for cluster-scoped instances.
type ClusterClientCredentialsSpec struct {
	// ClientID, when set, overrides secretRef.clientIdKey.
	// +optional
	ClientID *string `json:"clientId,omitempty"`

	// +kubebuilder:validation:Required
	SecretRef ClusterClientCredentialsSecretRefSpec `json:"secretRef"`
}

// ClusterClientCredentialsSecretRefSpec references a client-credentials Secret.
// Namespace is required because the resource is cluster-scoped.
type ClusterClientCredentialsSecretRefSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +kubebuilder:default="client-id"
	// +optional
	ClientIdKey string `json:"clientIdKey,omitempty"`

	// +kubebuilder:default="client-secret"
	// +optional
	ClientSecretKey string `json:"clientSecretKey,omitempty"`
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
