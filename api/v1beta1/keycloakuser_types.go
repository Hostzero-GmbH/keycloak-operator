package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakUserSpec defines the desired state of KeycloakUser
type KeycloakUserSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef, clusterRealmRef, or clientRef must be specified
	// Use this for regular realm users
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef, clusterRealmRef, or clientRef must be specified
	// Use this for regular realm users with cluster-scoped realms
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ClientRef is a reference to a KeycloakClient for service account users
	// One of realmRef, clusterRealmRef, or clientRef must be specified
	// Use this to manage the service account user associated with a client
	// +optional
	ClientRef *ResourceRef `json:"clientRef,omitempty"`

	// Definition contains the Keycloak UserRepresentation
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Definition *runtime.RawExtension `json:"definition,omitempty"`

	// InitialPassword sets the initial password for the user (only on creation)
	// +optional
	InitialPassword *InitialPassword `json:"initialPassword,omitempty"`

	// UserSecret configures where to store user credentials
	// +optional
	UserSecret *UserSecretSpec `json:"userSecret,omitempty"`
}

// InitialPassword defines the initial password for a user
type InitialPassword struct {
	// Value is the password value
	Value string `json:"value"`

	// Temporary indicates if the user must change password on first login
	// +optional
	Temporary bool `json:"temporary,omitempty"`
}

// UserSecretSpec defines where to store user credentials
type UserSecretSpec struct {
	// SecretName is the name of the Kubernetes secret to create
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// UsernameKey is the key for the username in the secret (defaults to "username")
	// +optional
	UsernameKey *string `json:"usernameKey,omitempty"`

	// PasswordKey is the key for the password (defaults to "password")
	// +optional
	PasswordKey *string `json:"passwordKey,omitempty"`

	// GeneratePassword indicates whether to generate a password
	// +optional
	GeneratePassword *bool `json:"generatePassword,omitempty"`
}

// UserDefinition represents the Keycloak UserRepresentation
// This is a subset - use runtime.RawExtension for full flexibility
type UserDefinition struct {
	// Username is the unique username
	// +kubebuilder:validation:Required
	Username string `json:"username"`

	// Email address
	// +optional
	Email string `json:"email,omitempty"`

	// EmailVerified indicates if email is verified
	// +optional
	EmailVerified *bool `json:"emailVerified,omitempty"`

	// FirstName of the user
	// +optional
	FirstName string `json:"firstName,omitempty"`

	// LastName of the user
	// +optional
	LastName string `json:"lastName,omitempty"`

	// Enabled indicates if the user is enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Groups the user belongs to
	// +optional
	Groups []string `json:"groups,omitempty"`

	// RealmRoles assigned to the user
	// +optional
	RealmRoles []string `json:"realmRoles,omitempty"`

	// ClientRoles assigned to the user (map of client to roles)
	// +optional
	ClientRoles map[string][]string `json:"clientRoles,omitempty"`

	// RequiredActions for the user
	// +optional
	RequiredActions []string `json:"requiredActions,omitempty"`

	// Attributes for custom user attributes
	// +optional
	Attributes map[string][]string `json:"attributes,omitempty"`

	// Credentials for the user (e.g., password)
	// +optional
	Credentials []CredentialRepresentation `json:"credentials,omitempty"`
}

// CredentialRepresentation represents user credentials
type CredentialRepresentation struct {
	// Type of credential (e.g., "password")
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Value of the credential
	// +optional
	Value string `json:"value,omitempty"`

	// Temporary indicates if the credential is temporary
	// +optional
	Temporary *bool `json:"temporary,omitempty"`
}

// KeycloakUserStatus defines the observed state of KeycloakUser
type KeycloakUserStatus struct {
	// Ready indicates if the user is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this user
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// UserID is the Keycloak internal user ID
	// +optional
	UserID string `json:"userID,omitempty"`

	// IsServiceAccount indicates if this user is a service account for a client
	// +optional
	IsServiceAccount bool `json:"isServiceAccount,omitempty"`

	// ClientID is the client UUID if this is a service account user
	// +optional
	ClientID string `json:"clientID,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the user is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcu,categories={keycloak,all}

// KeycloakUser defines a user within a KeycloakRealm
type KeycloakUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakUserSpec   `json:"spec,omitempty"`
	Status KeycloakUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakUserList contains a list of KeycloakUser
type KeycloakUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakUser{}, &KeycloakUserList{})
}

// GetRealmRef returns the realm reference (nil if using clusterRealmRef or clientRef)
func (u *KeycloakUser) GetRealmRef() *ResourceRef {
	return u.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef or clientRef)
func (u *KeycloakUser) GetClusterRealmRef() *ClusterResourceRef {
	return u.Spec.ClusterRealmRef
}

// GetClientRef returns the client reference for service account users
func (u *KeycloakUser) GetClientRef() *ResourceRef {
	return u.Spec.ClientRef
}

// UsesClusterRealm returns true if this user references a ClusterKeycloakRealm
func (u *KeycloakUser) UsesClusterRealm() bool {
	return u.Spec.ClusterRealmRef != nil
}

// IsServiceAccountUser returns true if this user is a service account for a client
func (u *KeycloakUser) IsServiceAccountUser() bool {
	return u.Spec.ClientRef != nil
}
