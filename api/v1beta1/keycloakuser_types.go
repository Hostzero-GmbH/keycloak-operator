package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakUserSpec defines the desired state of KeycloakUser
// +kubebuilder:validation:XValidation:rule="(has(self.realmRef) ? 1 : 0) + (has(self.clusterRealmRef) ? 1 : 0) + (has(self.clientRef) ? 1 : 0) == 1",message="exactly one of realmRef, clusterRealmRef, or clientRef must be set"
// +kubebuilder:validation:XValidation:rule="has(self.clientRef) || (has(self.username) && size(self.username) > 0)",message="spec.username is required unless spec.clientRef is set (service account user)"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.username) || (has(self.username) && self.username == oldSelf.username)",message="spec.username is immutable once set"
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

	// Username is the username in Keycloak. Required for regular realm users;
	// omit it for service account users, which are identified by clientRef and
	// whose username is derived by Keycloak. Immutable once set.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Username *string `json:"username,omitempty"`

	// Definition contains the Keycloak UserRepresentation. Set the username via
	// spec.username; role and group assignments go in spec.realmRoles,
	// spec.clientRoles, and spec.groups.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Definition *runtime.RawExtension `json:"definition,omitempty"`

	// RealmRoles is the authoritative set of realm-level role names for this
	// user, reconciled via the Keycloak role-mapping endpoints. When omitted,
	// realm roles are not managed; an empty list removes all realm roles.
	// Pointer types so an explicit empty value survives JSON round-trips.
	// Do not combine with KeycloakRoleMapping resources targeting the same user.
	// +optional
	RealmRoles *[]string `json:"realmRoles,omitempty"`

	// ClientRoles maps a client's clientId to the authoritative set of
	// client-level role names for this user. When omitted, client roles are not
	// managed; when set, roles on clients absent from the map are removed.
	// Do not combine with KeycloakRoleMapping resources targeting the same user.
	// +optional
	ClientRoles *map[string][]string `json:"clientRoles,omitempty"`

	// Groups is the authoritative set of group names this user belongs to,
	// reconciled via the Keycloak group-membership endpoints. When omitted,
	// group memberships are not managed; an empty list removes all memberships.
	// +optional
	Groups *[]string `json:"groups,omitempty"`

	// InitialPassword sets the initial password for the user (only on creation).
	// For managed credentials stored in a Kubernetes secret, use KeycloakUserCredential.
	// +optional
	InitialPassword *InitialPassword `json:"initialPassword,omitempty"`
}

// InitialPassword defines the initial password for a user
type InitialPassword struct {
	// Value is the password value
	Value string `json:"value"`

	// Temporary indicates if the user must change password on first login
	// +optional
	Temporary bool `json:"temporary,omitempty"`
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

	// Username is the resolved username in Keycloak
	// +optional
	Username string `json:"username,omitempty"`

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
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.status.username`,description="Username in Keycloak"
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
