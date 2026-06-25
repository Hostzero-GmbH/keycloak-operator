package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakIdentityProviderSpec defines the desired state of KeycloakIdentityProvider
// +kubebuilder:validation:XValidation:rule="has(self.realmRef) != has(self.clusterRealmRef)",message="exactly one of realmRef or clusterRealmRef must be set"
type KeycloakIdentityProviderSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ConfigSecretRef is a reference to a Kubernetes Secret whose data entries
	// are merged into definition.config before syncing to Keycloak. This allows
	// sensitive configuration values (e.g. clientId, clientSecret) to be stored
	// in a Secret rather than in plaintext in the CR. Secret values take
	// precedence over values specified inline in definition.config.
	// +optional
	ConfigSecretRef *IDPConfigSecretRef `json:"configSecretRef,omitempty"`

	// TokenExchange configures fine-grained-authz so that exactly the listed
	// clients (and no others) may exchange tokens with this IdP as
	// `subject_issuer`. Omit the field to leave token-exchange permissions
	// unmanaged (whatever was clicked manually stays). Set to a list (possibly
	// empty) to have the operator enable IdP permissions and bind a Client-type
	// policy listing the allowed clients on the `token-exchange` scope
	// permission in the realm-management authz resource server.
	// +optional
	TokenExchange *IDPTokenExchangeSpec `json:"tokenExchange,omitempty"`

	// Definition contains the Keycloak IdentityProviderRepresentation
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Definition runtime.RawExtension `json:"definition"`
}

// IDPTokenExchangeSpec configures who may perform RFC 8693 Token Exchange
// using this identity provider as `subject_issuer`. The operator translates
// this into Keycloak fine-grained-authz primitives (realm-management authz
// resource server + scope permission on the IdP).
type IDPTokenExchangeSpec struct {
	// AllowedClients is the list of clientIds (text, not UUIDs) in the same
	// realm as the IdP that are permitted to perform token-exchange against
	// this IdP. An empty list creates a policy that matches no clients,
	// effectively denying all (useful as an explicit lockdown). Omitting the
	// parent `tokenExchange` field entirely leaves Keycloak permissions
	// untouched.
	// +kubebuilder:validation:Required
	AllowedClients []string `json:"allowedClients"`
}

// IDPConfigSecretRef references a Kubernetes Secret containing identity provider
// configuration values to be merged into definition.config.
type IDPConfigSecretRef struct {
	// Name of the Kubernetes Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// IDPTokenExchangeStatus records the observed wiring of the token-exchange
// fine-grained-authz primitives. The operator manages a Client-type policy
// in the realm-management authz resource server and binds it to the auto-
// created scope permission on the IdP.
type IDPTokenExchangeStatus struct {
	// Enabled reflects whether fine-grained authz permissions are enabled on
	// this IdP in Keycloak.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// PermissionID is the ID of the `token-exchange` scope permission auto-
	// created in the realm-management authz resource server when permissions
	// are enabled on this IdP.
	// +optional
	PermissionID string `json:"permissionID,omitempty"`

	// PolicyID is the ID of the Client-type authz policy managed by the
	// operator (carries the AllowedClients list).
	// +optional
	PolicyID string `json:"policyID,omitempty"`

	// PolicyName is the name of the managed policy, useful for admins
	// looking the resource up in the Keycloak UI.
	// +optional
	PolicyName string `json:"policyName,omitempty"`

	// Message carries the last token-exchange reconcile error, if any. Set
	// only when token-exchange reconcile fails — the parent `status.ready`
	// still reflects the IdP itself, not the TE side.
	// +optional
	Message string `json:"message,omitempty"`
}

// KeycloakIdentityProviderStatus defines the observed state of KeycloakIdentityProvider
type KeycloakIdentityProviderStatus struct {
	// Ready indicates if the identity provider is ready
	Ready bool `json:"ready"`

	// Status is a human-readable status message
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information
	// +optional
	Message string `json:"message,omitempty"`

	// ResourcePath is the Keycloak API path for this identity provider
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// TokenExchange contains the observed state of the token-exchange
	// permission wiring, populated only when spec.tokenExchange is set.
	// +optional
	TokenExchange *IDPTokenExchangeStatus `json:"tokenExchange,omitempty"`

	// Instance contains the resolved instance reference
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the identity provider is ready"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`,description="Status message"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcidp,categories={keycloak,all}

// KeycloakIdentityProvider defines an identity provider within a KeycloakRealm
type KeycloakIdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakIdentityProviderSpec   `json:"spec,omitempty"`
	Status KeycloakIdentityProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakIdentityProviderList contains a list of KeycloakIdentityProvider
type KeycloakIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakIdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakIdentityProvider{}, &KeycloakIdentityProviderList{})
}
