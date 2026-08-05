package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakClientSpec defines the desired state of KeycloakClient
// +kubebuilder:validation:XValidation:rule="has(self.realmRef) != has(self.clusterRealmRef)",message="exactly one of realmRef or clusterRealmRef must be set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.clientId) || self.clientId == oldSelf.clientId",message="spec.clientId is immutable once set"
type KeycloakClientSpec struct {
	// RealmRef is a reference to a KeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm
	// One of realmRef or clusterRealmRef must be specified
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// ClientId is the client ID in Keycloak. Immutable once set.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientId *string `json:"clientId,omitempty"`

	// Definition contains the Keycloak ClientRepresentation. Set the client ID
	// via spec.clientId.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Definition *runtime.RawExtension `json:"definition,omitempty"`

	// ClientSecretRef configures the Kubernetes Secret for client credentials.
	// If the secret exists, its value is used. If it doesn't exist and Create is true,
	// the operator auto-generates a secret and creates it.
	// For public clients (publicClient: true) the Secret is still materialised
	// when ClientSecretRef is set, but only contains the client-id key — there
	// is no client_secret to store.
	// +optional
	ClientSecretRef *ClientSecretRefSpec `json:"clientSecretRef,omitempty"`
}

// ClientSecretRefSpec references a Kubernetes Secret for the client credentials.
// If the secret exists, its value is used. If it doesn't exist and Create is true,
// the operator auto-generates a secret and creates it.
// For public clients the Secret will only contain the client-id key.
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

	// ClientID is the resolved client ID (clientId) in Keycloak
	// +optional
	ClientID string `json:"clientId,omitempty"`

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
// +kubebuilder:printcolumn:name="ClientId",type=string,JSONPath=`.status.clientId`,description="Client ID in Keycloak"
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
