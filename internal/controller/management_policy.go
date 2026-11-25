package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

const (
	// DriftedConditionType is set on CRs with managementPolicy Observe. It
	// reports whether spec.definition matches the live Keycloak resource. It
	// never affects Ready, and it is removed in Manage mode, where drift is
	// corrected instead of reported.
	DriftedConditionType = "Drifted"

	// ObservedReason marks an Observe-mode CR whose Keycloak resource exists.
	ObservedReason = "Observed"
	// NotFoundReason marks an Observe-mode CR whose Keycloak resource does not
	// exist. The operator does not create it.
	NotFoundReason = "NotFound"
	// LookupFailedReason marks a lookup error other than "not found", so that
	// an outage is not reported as a missing resource.
	LookupFailedReason = "LookupFailed"

	// DefinitionMismatchReason is the Drifted=True reason.
	DefinitionMismatchReason = "DefinitionMismatch"
	// InSyncReason is the Drifted=False reason.
	InSyncReason = "InSync"

	// maxReportedDriftKeys bounds the key list in the Drifted message.
	maxReportedDriftKeys = 10
)

// definitionMatcher compares a desired definition against the live
// representation, with the same semantics as the Manage-mode drift detection
// of the resource kind (for example realmDefinitionsMatch).
type definitionMatcher func(desired, current json.RawMessage) bool

// observation is the outcome of an Observe-mode lookup.
type observation struct {
	// Ready is true when the Keycloak resource exists.
	Ready   bool
	Reason  string
	Message string
	// Drifted lists the top-level definition keys that differ from the live
	// resource. It is meaningful only when Ready is true.
	Drifted []string
}

// observe turns the result of a lookup into an observation. kind and name
// identify the resource in messages (for example "realm", "my-realm").
// It never writes to Keycloak: in Observe mode the caller returns right after
// recording the observation.
func observe(kind, name string, desired, current json.RawMessage, lookupErr error, match definitionMatcher) observation {
	if lookupErr != nil {
		if keycloak.IsNotFound(lookupErr) {
			return observation{
				Reason:  NotFoundReason,
				Message: fmt.Sprintf("%s %q does not exist in Keycloak; managementPolicy Observe never creates it", kind, name),
			}
		}
		return observation{
			Reason:  LookupFailedReason,
			Message: fmt.Sprintf("Failed to look up %s %q: %v", kind, name, lookupErr),
		}
	}
	return observation{
		Ready:   true,
		Reason:  ObservedReason,
		Message: fmt.Sprintf("%s %q exists in Keycloak; managementPolicy Observe, no changes are applied", kind, name),
		Drifted: driftedKeys(desired, current, match),
	}
}

// driftedKeys returns the sorted top-level keys of desired whose value does
// not match current. Each key is compared on its own with match, so the
// mask-aware rules of the matcher (for example smtpServer.password) still
// apply. An unparsable desired definition is reported as a whole.
func driftedKeys(desired, current json.RawMessage, match definitionMatcher) []string {
	var desiredMap map[string]json.RawMessage
	if err := json.Unmarshal(desired, &desiredMap); err != nil {
		return []string{"definition"}
	}

	var keys []string
	for key, value := range desiredMap {
		single, err := json.Marshal(map[string]json.RawMessage{key: value})
		if err != nil || !match(single, current) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// setDriftedCondition records the drift outcome of an observation.
func setDriftedCondition(conditions *[]metav1.Condition, drifted []string) {
	condition := metav1.Condition{
		Type:    DriftedConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  InSyncReason,
		Message: "spec.definition matches the live resource",
	}
	if len(drifted) > 0 {
		condition.Status = metav1.ConditionTrue
		condition.Reason = DefinitionMismatchReason
		condition.Message = "spec.definition differs from the live resource on: " + formatKeys(drifted)
	}
	// SetStatusCondition keeps LastTransitionTime while the status does not
	// flip, which keeps steady-state status writes away (see setReadyCondition).
	meta.SetStatusCondition(conditions, condition)
}

// removeDriftedCondition drops the Drifted condition. Manage mode corrects
// drift, so the condition has no meaning there.
func removeDriftedCondition(conditions *[]metav1.Condition) {
	meta.RemoveStatusCondition(conditions, DriftedConditionType)
}

func formatKeys(keys []string) string {
	if len(keys) <= maxReportedDriftKeys {
		return strings.Join(keys, ", ")
	}
	return fmt.Sprintf("%s (and %d more)", strings.Join(keys[:maxReportedDriftKeys], ", "), len(keys)-maxReportedDriftKeys)
}

// observeRealm reads a realm for managementPolicy Observe. raw is the CR's
// spec.definition; the resolved realm name is injected the same way as in
// Manage mode, so both modes compare the same desired representation.
// smtpSecretRef is not resolved: credentials are never pushed in Observe mode,
// and Keycloak masks the stored password anyway.
func observeRealm(ctx context.Context, kc *keycloak.Client, raw []byte, realmName string) observation {
	desired := json.RawMessage(raw)
	if len(desired) == 0 {
		desired = json.RawMessage("{}")
	}
	desired = setFieldInDefinition(desired, "realm", realmName)
	current, err := kc.GetRealmRaw(ctx, realmName)
	return observe("realm", realmName, desired, current, err, realmDefinitionsMatch)
}

// errorLabel returns the metrics error label of a failed observation.
func (o observation) errorLabel() string {
	if o.Reason == NotFoundReason {
		return "observed_resource_not_found"
	}
	return "keycloak_api_error"
}

// recordObservation applies an observation to a status: resource path and
// Drifted condition. Ready, reason and message are set by the caller's
// updateStatus.
func recordObservation(obs observation, conditions *[]metav1.Condition, resourcePath *string, path string) {
	if !obs.Ready {
		removeDriftedCondition(conditions)
		return
	}
	*resourcePath = path
	setDriftedCondition(conditions, obs.Drifted)
}
