package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// KeycloakAuthenticationFlowSpec defines the desired state of KeycloakAuthenticationFlow
// +kubebuilder:validation:XValidation:rule="has(self.realmRef) != has(self.clusterRealmRef)",message="exactly one of realmRef or clusterRealmRef must be set"
type KeycloakAuthenticationFlowSpec struct {
	// RealmRef is a reference to a KeycloakRealm.
	// One of realmRef or clusterRealmRef must be specified.
	// +optional
	RealmRef *ResourceRef `json:"realmRef,omitempty"`

	// ClusterRealmRef is a reference to a ClusterKeycloakRealm.
	// One of realmRef or clusterRealmRef must be specified.
	// +optional
	ClusterRealmRef *ClusterResourceRef `json:"clusterRealmRef,omitempty"`

	// Alias is the unique identifier for this flow within the realm.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Alias string `json:"alias"`

	// Description is a human-readable description of the flow.
	// +optional
	Description string `json:"description,omitempty"`

	// ProviderId is the top-level flow type. Keycloak ships with "basic-flow"
	// and "client-flow"; sub-flows may additionally use "form-flow".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProviderId string `json:"providerId"`

	// Executions is the ordered list of executions for this flow as a JSON
	// array. Each entry is either a leaf authenticator or a nested sub-flow.
	//
	// Leaf authenticator (object fields):
	//
	//   authenticator:        Keycloak provider ID, e.g. "auth-cookie".
	//   requirement:          REQUIRED | ALTERNATIVE | DISABLED | CONDITIONAL.
	//   authenticatorConfig:  optional map[string]string applied to the
	//                         execution after creation.
	//
	// Sub-flow (object fields):
	//
	//   subFlow:              { alias, providerId, description? } — the
	//                         child flow definition. providerId is typically
	//                         "basic-flow" or "form-flow"; "form-flow" is
	//                         required when the children are FormAction
	//                         providers (e.g. registration-user-creation).
	//   requirement:          REQUIRED | ALTERNATIVE | DISABLED | CONDITIONAL.
	//   executions:           ordered list of child executions, recursively
	//                         using the same shape. As a convenience, child
	//                         executions may also be placed inside
	//                         subFlow.executions; if both are present, the
	//                         inline list precedes the sibling list.
	//
	// Nesting depth is unconstrained.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Executions runtime.RawExtension `json:"executions,omitempty"`
}

// KeycloakAuthenticationFlowStatus defines the observed state of KeycloakAuthenticationFlow
type KeycloakAuthenticationFlowStatus struct {
	// Ready indicates if the flow is synchronized with Keycloak.
	Ready bool `json:"ready"`

	// Status is a human-readable status message.
	// +optional
	Status string `json:"status,omitempty"`

	// Message contains additional information.
	// +optional
	Message string `json:"message,omitempty"`

	// FlowID is the Keycloak internal ID of the top-level flow.
	// +optional
	FlowID string `json:"flowID,omitempty"`

	// ResourcePath is the Keycloak API path for this flow.
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// Instance contains the resolved instance reference.
	// +optional
	Instance *InstanceRef `json:"instance,omitempty"`

	// Realm contains the resolved realm reference.
	// +optional
	Realm *RealmRef `json:"realm,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`,description="Whether the flow is ready"
// +kubebuilder:printcolumn:name="Alias",type=string,JSONPath=`.spec.alias`,description="Flow alias"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=kcaf,categories={keycloak,all}

// KeycloakAuthenticationFlow manages a Keycloak authentication flow.
type KeycloakAuthenticationFlow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeycloakAuthenticationFlowSpec   `json:"spec,omitempty"`
	Status KeycloakAuthenticationFlowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeycloakAuthenticationFlowList contains a list of KeycloakAuthenticationFlow
type KeycloakAuthenticationFlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeycloakAuthenticationFlow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeycloakAuthenticationFlow{}, &KeycloakAuthenticationFlowList{})
}

// GetRealmRef returns the realm reference (nil if using clusterRealmRef)
func (f *KeycloakAuthenticationFlow) GetRealmRef() *ResourceRef {
	return f.Spec.RealmRef
}

// GetClusterRealmRef returns the cluster realm reference (nil if using realmRef)
func (f *KeycloakAuthenticationFlow) GetClusterRealmRef() *ClusterResourceRef {
	return f.Spec.ClusterRealmRef
}

// UsesClusterRealm returns true if this flow references a ClusterKeycloakRealm
func (f *KeycloakAuthenticationFlow) UsesClusterRealm() bool {
	return f.Spec.ClusterRealmRef != nil
}
