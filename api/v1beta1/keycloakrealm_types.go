package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceRef is a reference to another resource
type ResourceRef struct {
	// Name of the resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the resource (optional, defaults to the same namespace)
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// ClusterResourceRef is a reference to a cluster-scoped resource
type ClusterResourceRef struct {
	// Name of the cluster-scoped resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// KeycloakRealmSpec defines the desired state of KeycloakRealm
type KeycloakRealmSpec struct {
	// InstanceRef is a reference to a KeycloakInstance
	// One of instanceRef or clusterInstanceRef must be specified
	// +optional
	InstanceRef *ResourceRef `json:"instanceRef,omitempty"`

	// ClusterInstanceRef is a reference to a ClusterKeycloakInstance
	// One of instanceRef or clusterInstanceRef must be specified
	// +optional
	ClusterInstanceRef *ClusterResourceRef `json:"clusterInstanceRef,omitempty"`

	// RealmName is the name of the realm in Keycloak (defaults to metadata.name)
	// +optional
	RealmName *string `json:"realmName,omitempty"`

	// Definition contains the Keycloak RealmRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// RealmDefinition represents the Keycloak RealmRepresentation
// This is a subset of the full representation - use runtime.RawExtension for full flexibility
type RealmDefinition struct {
	// Realm is the realm name (unique identifier)
	// +kubebuilder:validation:Required
	Realm string `json:"realm"`

	// DisplayName is the display name of the realm
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// DisplayNameHtml is the HTML display name
	// +optional
	DisplayNameHtml string `json:"displayNameHtml,omitempty"`

	// Enabled indicates whether the realm is enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// RegistrationAllowed allows user self-registration
	// +optional
	RegistrationAllowed *bool `json:"registrationAllowed,omitempty"`

	// LoginWithEmailAllowed allows login with email
	// +optional
	LoginWithEmailAllowed *bool `json:"loginWithEmailAllowed,omitempty"`

	// DuplicateEmailsAllowed allows duplicate emails
	// +optional
	DuplicateEmailsAllowed *bool `json:"duplicateEmailsAllowed,omitempty"`

	// ResetPasswordAllowed enables password reset
	// +optional
	ResetPasswordAllowed *bool `json:"resetPasswordAllowed,omitempty"`

	// VerifyEmail requires email verification
	// +optional
	VerifyEmail *bool `json:"verifyEmail,omitempty"`

	// SslRequired specifies SSL requirement level
	// +optional
	// +kubebuilder:validation:Enum=all;external;none
	SslRequired string `json:"sslRequired,omitempty"`

	// AccessTokenLifespan in seconds
	// +optional
	AccessTokenLifespan *int32 `json:"accessTokenLifespan,omitempty"`

	// SsoSessionIdleTimeout in seconds
	// +optional
	SsoSessionIdleTimeout *int32 `json:"ssoSessionIdleTimeout,omitempty"`

	// SsoSessionMaxLifespan in seconds
	// +optional
	SsoSessionMaxLifespan *int32 `json:"ssoSessionMaxLifespan,omitempty"`

	// LoginTheme for the realm
	// +optional
	LoginTheme string `json:"loginTheme,omitempty"`

	// AccountTheme for the realm
	// +optional
	AccountTheme string `json:"accountTheme,omitempty"`

	// AdminTheme for the realm
	// +optional
	AdminTheme string `json:"adminTheme,omitempty"`

	// EmailTheme for the realm
	// +optional
	EmailTheme string `json:"emailTheme,omitempty"`

	// InternationalizationEnabled enables i18n
	// +optional
	InternationalizationEnabled *bool `json:"internationalizationEnabled,omitempty"`

	// SupportedLocales list of supported locales
	// +optional
	SupportedLocales []string `json:"supportedLocales,omitempty"`

	// DefaultLocale for the realm
	// +optional
	DefaultLocale string `json:"defaultLocale,omitempty"`

	// BruteForceProtected enables brute force protection
	// +optional
	BruteForceProtected *bool `json:"bruteForceProtected,omitempty"`

	// SmtpServer configuration
	// +optional
	SmtpServer map[string]string `json:"smtpServer,omitempty"`

	// Attributes for custom attributes
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
}

// KeycloakRealmStatus defines the observed state of KeycloakRealm
type KeycloakRealmStatus struct {
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

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InstanceRef contains the resolved instance reference
type InstanceRef struct {
	// InstanceRef is the name of the namespaced instance
	// +optional
	InstanceRef string `json:"instanceRef,omitempty"`

	// ClusterInstanceRef is the name of the cluster instance
	// +optional
	ClusterInstanceRef string `json:"clusterInstanceRef,omitempty"`
}

// RealmRef contains the resolved realm reference
type RealmRef struct {
	// RealmRef is the name of the namespaced realm
	// +optional
	RealmRef string `json:"realmRef,omitempty"`

	// ClusterRealmRef is the name of the cluster realm
	// +optional
	ClusterRealmRef string `json:"clusterRealmRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the realm is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcrm,categories={keycloak,all}

// KeycloakRealm defines a realm within a KeycloakInstance
type KeycloakRealm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakRealmSpec   `json:"spec,omitempty"`
	Status KeycloakRealmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakRealmList contains a list of KeycloakRealm
type KeycloakRealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakRealm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakRealm{}, &KeycloakRealmList{})
}

// GetInstanceRef returns the instance reference (nil if using clusterInstanceRef)
func (r *KeycloakRealm) GetInstanceRef() *ResourceRef {
	return r.Spec.InstanceRef
}

// GetClusterInstanceRef returns the cluster instance reference (nil if using instanceRef)
func (r *KeycloakRealm) GetClusterInstanceRef() *ClusterResourceRef {
	return r.Spec.ClusterInstanceRef
}

// UsesClusterInstance returns true if this realm references a ClusterKeycloakInstance
func (r *KeycloakRealm) UsesClusterInstance() bool {
	return r.Spec.ClusterInstanceRef != nil
}

// GetRealmName returns the realm name to use in Keycloak
func (r *KeycloakRealm) GetRealmName() string {
	if r.Spec.RealmName != nil {
		return *r.Spec.RealmName
	}
	return r.Name
}
