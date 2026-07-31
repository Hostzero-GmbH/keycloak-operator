package controller

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ReadyConditionType = "Ready"

// setReadyCondition adds or updates the "Ready" condition in the supplied slice,
// preserving LastTransitionTime while the condition status does not flip.
//
// The timestamp must not be refreshed on every pass: a status write that only
// moves LastTransitionTime still bumps resourceVersion, which fires the
// controller's own watch and re-enqueues the object immediately instead of
// honouring the sync period.
func setReadyCondition(conditions []metav1.Condition, ready bool, reason, message string) []metav1.Condition {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	condition := metav1.Condition{
		Type:               ReadyConditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	for i, c := range conditions {
		if c.Type == ReadyConditionType {
			if c.Status == status {
				condition.LastTransitionTime = c.LastTransitionTime
			}
			conditions[i] = condition
			return conditions
		}
	}
	return append(conditions, condition)
}

// writeStatusIfChanged persists obj's status only when it differs from what the
// API server already holds, then returns the requeue result for the reconcile
// outcome.
//
// Skipping no-op writes is what keeps a steady-state resource quiescent; see
// setReadyCondition for why an unconditional write self-triggers.
func writeStatusIfChanged(ctx context.Context, c client.Client, obj client.Object, ready bool) (ctrl.Result, error) {
	if !statusMatchesStored(ctx, c, obj) {
		if err := c.Status().Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// statusMatchesStored reports whether obj's in-memory status already equals the
// stored one. The comparison reads through the informer cache rather than
// snapshotting at the top of updateStatus, because reconcilers routinely set
// status fields (ResourcePath, PasswordHash, …) before calling it — a snapshot
// taken here would classify those as already persisted and silently drop them.
//
// Anything unexpected (read error, no Status field) is reported as a mismatch
// so the write still goes ahead.
func statusMatchesStored(ctx context.Context, c client.Client, obj client.Object) bool {
	stored, ok := obj.DeepCopyObject().(client.Object)
	if !ok {
		return false
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), stored); err != nil {
		return false
	}

	storedStatus, currentStatus := statusOf(stored), statusOf(obj)
	if storedStatus == nil || currentStatus == nil {
		return false
	}
	return equality.Semantic.DeepEqual(storedStatus, currentStatus)
}

// statusOf returns the Status field of a Keycloak CR, or nil if the type has none.
func statusOf(obj client.Object) interface{} {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	field := v.Elem().FieldByName("Status")
	if !field.IsValid() {
		return nil
	}
	return field.Interface()
}
