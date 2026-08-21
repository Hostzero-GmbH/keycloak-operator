package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// KeycloakRequiredActionReconciler reconciles a KeycloakRequiredAction object
type KeycloakRequiredActionReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrequiredactions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrequiredactions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrequiredactions/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles KeycloakRequiredAction reconciliation
func (r *KeycloakRequiredActionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakRequiredAction"

	ra := &keycloakv1beta1.KeycloakRequiredAction{}
	if err := r.Get(ctx, req.NamespacedName, ra); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRequiredAction")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	defer func() {
		RecordReconcile(controllerName, ra.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !ra.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ra, FinalizerName) {
			if ShouldPreserveResource(ra) {
				log.Info("preserving required action in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteRequiredAction(ctx, ra); err != nil {
				log.Error(err, "failed to delete required action from Keycloak")
			}

			controllerutil.RemoveFinalizer(ra, FinalizerName)
			if err := r.Update(ctx, ra); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(ra, FinalizerName) {
		controllerutil.AddFinalizer(ra, FinalizerName)
		if err := r.Update(ctx, ra); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, ra)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, ra, false, "RealmNotReady", err.Error(), "")
	}

	// Parse definition to extract alias
	var raDef struct {
		Alias      string `json:"alias"`
		ProviderID string `json:"providerId"`
	}
	if err := json.Unmarshal(ra.Spec.Definition.Raw, &raDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, ra, false, "InvalidDefinition", fmt.Sprintf("Failed to parse definition: %v", err), "")
	}

	// Resolve the alias from spec.alias.
	alias, err := resolveIdentifier("alias", ra.Spec.Alias, raDef.Alias)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, ra, false, InvalidIdentifierReason, err.Error(), "")
	}

	definition := setFieldInDefinition(ra.Spec.Definition.Raw, "alias", alias)

	definition, err = applyConfigSecret(ctx, r.Client, ra.Namespace, ra.Spec.ConfigSecretRef, definition, false)
	if err != nil {
		RecordError(controllerName, "secret_error")
		return r.updateStatus(ctx, ra, false, "ConfigSecretError", err.Error(), alias)
	}

	// Check if the required action already exists
	existing, err := kc.GetRequiredAction(ctx, realmName, alias)

	if err != nil || existing == nil {
		// Required action doesn't exist -- register it first, then update
		log.Info("registering required action", "alias", alias, "realm", realmName)

		providerID := raDef.ProviderID
		if providerID == "" {
			providerID = alias
		}

		registerPayload, _ := json.Marshal(map[string]string{
			"providerId": providerID,
			"name":       alias,
		})
		if err := kc.RegisterRequiredAction(ctx, realmName, registerPayload); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, ra, false, "RegisterFailed", fmt.Sprintf("Failed to register required action: %v", err), "")
		}

		// Now update it with the full definition
		if err := kc.UpdateRequiredAction(ctx, realmName, alias, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, ra, false, "UpdateFailed", fmt.Sprintf("Failed to configure required action after registration: %v", err), alias)
		}
		log.Info("required action registered and configured", "alias", alias)
	} else {
		// Required action exists -- update it only when it actually drifted.
		// Every PUT produces a Keycloak admin event, so an unconditional write
		// floods admin_event_entity for actions that never change.
		needsUpdate := true
		if currentRaw, fetchErr := kc.GetRequiredActionRaw(ctx, realmName, alias); fetchErr == nil {
			needsUpdate = !definitionsMatch(definition, currentRaw)
		}

		if needsUpdate {
			log.Info("updating required action", "alias", alias, "realm", realmName)
			if err := kc.UpdateRequiredAction(ctx, realmName, alias, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, ra, false, "UpdateFailed", fmt.Sprintf("Failed to update required action: %v", err), alias)
			}
			log.Info("required action updated", "alias", alias)
		} else {
			log.V(1).Info("required action already in sync, skipping update", "alias", alias, "realm", realmName)
		}
	}

	ra.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/authentication/required-actions/%s", realmName, alias)
	return r.updateStatus(ctx, ra, true, "Ready", "Required action synchronized", alias)
}

func (r *KeycloakRequiredActionReconciler) deleteRequiredAction(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, ra)
	if err != nil {
		return err
	}

	// Use spec.alias so deletion targets the synchronized required action.
	// Empty means never synchronized (unmigrated object).
	alias := identifierValue(ra.Spec.Alias)
	if alias == "" {
		return nil
	}
	return kc.DeleteRequiredAction(ctx, realmName, alias)
}

func (r *KeycloakRequiredActionReconciler) getKeycloakClientAndRealm(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction) (*keycloak.Client, string, error) {
	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, ra.Namespace, ra.Spec.RealmRef, ra.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", err
	}
	return res.Client, res.RealmName, nil
}

func (r *KeycloakRequiredActionReconciler) updateStatus(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction, ready bool, status, message, alias string) (ctrl.Result, error) {
	ra.Status.Ready = ready
	ra.Status.Status = status
	ra.Status.Message = message
	ra.Status.Alias = alias

	if ready {
		ra.Status.ObservedGeneration = ra.Generation
	}

	ra.Status.Conditions = setReadyCondition(ra.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, ra, ready)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRequiredActionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRequiredAction{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findRequiredActionsForSecret),
		).
		Complete(r)
}

func (r *KeycloakRequiredActionReconciler) findRequiredActionsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	return findForConfigSecret(ctx, r.Client, obj.(*corev1.Secret), &keycloakv1beta1.KeycloakRequiredActionList{}, func(o client.Object) *keycloakv1beta1.ConfigSecretRef {
		return o.(*keycloakv1beta1.KeycloakRequiredAction).Spec.ConfigSecretRef
	})
}
