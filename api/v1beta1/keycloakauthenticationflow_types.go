package v1beta1

// NOTE: Unlike other CRDs in this operator, KeycloakAuthenticationFlow uses a typed
// spec instead of runtime.RawExtension. Most CRDs pass a raw JSON definition through
// to a single Keycloak REST endpoint. Authentication flows cannot follow that pattern
// because Keycloak's flow API is procedural -- there is no single endpoint that accepts
// a complete flow definition. The controller must translate the spec into a sequence of
// API calls (create flow, add executions, set requirements, reorder, apply authenticator
// configs). A typed spec provides schema validation and makes the nested flow/execution
// structure natural to express in YAML.

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeycloakAuthenticationFlowSpec defines the desired state of KeycloakAuthenticationFlow
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

	// ProviderId is the flow type: "basic-flow" or "client-flow".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=basic-flow;client-flow
	ProviderId string `json:"providerId"`

	// Executions defines the ordered list of authenticator executions and sub-flows.
	// +optional
	Executions []AuthenticationExecution `json:"executions,omitempty"`
}

// AuthenticationExecution defines a single execution step within a flow.
// Exactly one of Authenticator or SubFlow must be set.
type AuthenticationExecution struct {
	// Authenticator is the provider ID (e.g. "auth-cookie", "auth-username-password-form").
	// Mutually exclusive with SubFlow.
	// +optional
	Authenticator string `json:"authenticator,omitempty"`

	// SubFlow defines a nested sub-flow. Mutually exclusive with Authenticator.
	// Sub-flow children are defined inside SubFlowDefinition.Executions.
	// +optional
	SubFlow *SubFlowDefinition `json:"subFlow,omitempty"`

	// Requirement is the execution requirement: REQUIRED, ALTERNATIVE, DISABLED, or CONDITIONAL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=REQUIRED;ALTERNATIVE;DISABLED;CONDITIONAL
	Requirement string `json:"requirement"`

	// AuthenticatorConfig contains key-value configuration for the authenticator.
	// Only valid when Authenticator is set.
	// +optional
	AuthenticatorConfig map[string]string `json:"authenticatorConfig,omitempty"`
}

// SubFlowDefinition identifies a sub-flow to be created within a parent flow.
// Child executions live here rather than on AuthenticationExecution to avoid a
// self-referential type, which Kubernetes CRD schemas do not support.
type SubFlowDefinition struct {
	// Alias is the unique identifier for this sub-flow.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Alias string `json:"alias"`

	// Description is a human-readable description of the sub-flow.
	// +optional
	Description string `json:"description,omitempty"`

	// ProviderId is the sub-flow type, typically "basic-flow".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=basic-flow;client-flow
	ProviderId string `json:"providerId"`

	// Executions defines the ordered list of authenticator executions within this sub-flow.
	// +optional
	Executions []SubFlowExecution `json:"executions,omitempty"`
}

// SubFlowExecution defines an authenticator execution within a sub-flow.
// Unlike AuthenticationExecution, this type does not support further nesting;
// Keycloak flows rarely exceed two levels in practice.
type SubFlowExecution struct {
	// Authenticator is the provider ID (e.g. "auth-username-password-form").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Authenticator string `json:"authenticator"`

	// Requirement is the execution requirement: REQUIRED, ALTERNATIVE, DISABLED, or CONDITIONAL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=REQUIRED;ALTERNATIVE;DISABLED;CONDITIONAL
	Requirement string `json:"requirement"`

	// AuthenticatorConfig contains key-value configuration for the authenticator.
	// +optional
	AuthenticatorConfig map[string]string `json:"authenticatorConfig,omitempty"`
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

// KeycloakAuthenticationFlow manages a Keycloak authentication flow via the
// procedural Admin REST API for flows and executions.
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
