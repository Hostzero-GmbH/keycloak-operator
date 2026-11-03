package v1beta1

// ManagementPolicy controls which operations the operator performs on the
// Keycloak resource a CR represents.
// +kubebuilder:validation:Enum=Manage;Observe
type ManagementPolicy string

const (
	// ManagementPolicyManage lets the operator create, update and delete the
	// Keycloak resource. This is the default.
	ManagementPolicyManage ManagementPolicy = "Manage"

	// ManagementPolicyObserve makes the operator read the Keycloak resource
	// only: it never creates, updates or deletes it. The CR becomes Ready when
	// the resource exists, and spec.definition is compared against the live
	// resource to report drift without correcting it. Use it when another
	// operator (for example on another cluster) owns a shared resource.
	ManagementPolicyObserve ManagementPolicy = "Observe"
)

// IsObserve reports whether the policy forbids writes to Keycloak. An empty
// value (objects stored before the field existed) means Manage.
func (p ManagementPolicy) IsObserve() bool {
	return p == ManagementPolicyObserve
}
