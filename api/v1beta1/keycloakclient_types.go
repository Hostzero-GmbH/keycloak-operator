package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakClientSpec defines the desired state of KeycloakClient
type KeycloakClientSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ClientId is the client ID in Keycloak (defaults to metadata.name)
	// +optional
	ClientId *string `json:"clientId,omitempty"`

	// Definition contains the Keycloak ClientRepresentation
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Definition *runtime.RawExtension `json:"definition,omitempty"`

	// ClientSecretRef configures the Kubernetes Secret for client credentials.
	// If the secret exists, its value is used. If it doesn't exist and Create is true,
	// the operator auto-generates a secret and creates it.
	// +optional
	ClientSecretRef *ClientSecretRefSpec `json:"clientSecretRef,omitempty"`
}

// ClientSecretRefSpec references a Kubernetes Secret for the client credentials.
// If the secret exists, its value is used. If it doesn't exist and Create is true,
// the operator auto-generates a secret and creates it.
type ClientSecretRefSpec struct {
	// Name of the Kubernetes Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ClientIdKey is the key for the client ID in the secret.
	// Defaults to "client-id".
	// +optional
	ClientIdKey *string `json:"clientIdKey,omitempty"`

	// ClientSecretKey is the key for the client secret value in the secret.
	// Defaults to "client-secret".
	// +optional
	ClientSecretKey *string `json:"clientSecretKey,omitempty"`

	// Create determines behavior when the secret doesn't exist.
	// If true (default): auto-generate a secret and create the Secret.
	// If false: error if the secret doesn't exist (strict mode for GitOps).
	// +optional
	// +kubebuilder:default=true
	Create *bool `json:"create,omitempty"`
}

// ClientDefinition represents the Keycloak ClientRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type ClientDefinition struct {
	// ClientId is the unique client identifier
	// +kubebuilder:validation:Required
	ClientId string `json:"clientId"`

	// Name is the display name of the client
	// +optional
	Name string `json:"name,omitempty"`

	// Description of the client
	// +optional
	Description string `json:"description,omitempty"`

	// Enabled indicates whether the client is enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Protocol is the client protocol (openid-connect or saml)
	// +optional
	// +kubebuilder:validation:Enum=openid-connect;saml
	Protocol string `json:"protocol,omitempty"`

	// PublicClient indicates if this is a public client
	// +optional
	PublicClient *bool `json:"publicClient,omitempty"`

	// StandardFlowEnabled enables Authorization Code Flow
	// +optional
	StandardFlowEnabled *bool `json:"standardFlowEnabled,omitempty"`

	// ImplicitFlowEnabled enables Implicit Flow
	// +optional
	ImplicitFlowEnabled *bool `json:"implicitFlowEnabled,omitempty"`

	// DirectAccessGrantsEnabled enables Direct Access Grants
	// +optional
	DirectAccessGrantsEnabled *bool `json:"directAccessGrantsEnabled,omitempty"`

	// ServiceAccountsEnabled enables service account
	// +optional
	ServiceAccountsEnabled *bool `json:"serviceAccountsEnabled,omitempty"`

	// BearerOnly indicates bearer-only client
	// +optional
	BearerOnly *bool `json:"bearerOnly,omitempty"`

	// ConsentRequired requires user consent
	// +optional
	ConsentRequired *bool `json:"consentRequired,omitempty"`

	// RootUrl is the root URL of the application
	// +optional
	RootUrl string `json:"rootUrl,omitempty"`

	// BaseUrl is the default URL for the client
	// +optional
	BaseUrl string `json:"baseUrl,omitempty"`

	// AdminUrl is the URL to the admin interface
	// +optional
	AdminUrl string `json:"adminUrl,omitempty"`

	// RedirectUris is a list of valid redirect URIs
	// +optional
	RedirectUris []string `json:"redirectUris,omitempty"`

	// WebOrigins is a list of allowed CORS origins
	// +optional
	WebOrigins []string `json:"webOrigins,omitempty"`

	// Secret is the client secret (for confidential clients)
	// +optional
	Secret string `json:"secret,omitempty"`

	// DefaultClientScopes assigned by default
	// +optional
	DefaultClientScopes []string `json:"defaultClientScopes,omitempty"`

	// OptionalClientScopes available optionally
	// +optional
	OptionalClientScopes []string `json:"optionalClientScopes,omitempty"`

	// Attributes for additional client configuration
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`

	// AuthorizationServicesEnabled enables fine-grained authorization
	// +optional
	AuthorizationServicesEnabled *bool `json:"authorizationServicesEnabled,omitempty"`

	// FullScopeAllowed allows full scope
	// +optional
	FullScopeAllowed *bool `json:"fullScopeAllowed,omitempty"`

	// FrontchannelLogout enables front-channel logout
	// +optional
	FrontchannelLogout *bool `json:"frontchannelLogout,omitempty"`

	// ClientAuthenticatorType specifies the authenticator type
	// +optional
	// +kubebuilder:validation:Enum=client-secret;client-jwt;client-secret-jwt;client-x509
	ClientAuthenticatorType string `json:"clientAuthenticatorType,omitempty"`
}

// KeycloakClientStatus defines the observed state of KeycloakClient
type KeycloakClientStatus struct {
	// Ready indicates if the client is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this client
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// ClientUUID is the Keycloak internal ID
	// +optional
	ClientUUID string `json:"clientUUID,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the client is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcc,categories={keycloak,all}

// KeycloakClient defines a client within a KeycloakRealm
type KeycloakClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakClientSpec   `json:"spec,omitempty"`
	Status KeycloakClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakClientList contains a list of KeycloakClient
type KeycloakClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakClient{}, &KeycloakClientList{})
}

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (c *KeycloakClient) GetRealmRef() *ResourceRef {
	return c.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (c *KeycloakClient) GetClusterRealmRef() *ClusterResourceRef {
	return c.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this client references a ClusterKeycloakRealm
func (c *KeycloakClient) UsesClusterRealm() bool {
	return c.Spec.ClusterRealmRef != nil
}
