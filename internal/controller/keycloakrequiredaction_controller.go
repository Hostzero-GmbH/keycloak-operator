package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

	alias := raDef.Alias
	if alias == "" {
		alias = ra.Name
	}

	definition := setFieldInDefinition(ra.Spec.Definition.Raw, "alias", alias)

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
		// Required action exists -- update it
		log.Info("updating required action", "alias", alias, "realm", realmName)
		if err := kc.UpdateRequiredAction(ctx, realmName, alias, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, ra, false, "UpdateFailed", fmt.Sprintf("Failed to update required action: %v", err), alias)
		}
		log.Info("required action updated", "alias", alias)
	}

	ra.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/authentication/required-actions/%s", realmName, alias)
	return r.updateStatus(ctx, ra, true, "Ready", "Required action synchronized", alias)
}

func (r *KeycloakRequiredActionReconciler) deleteRequiredAction(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, ra)
	if err != nil {
		return err
	}

	var raDef struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(ra.Spec.Definition.Raw, &raDef); err != nil {
		return fmt.Errorf("failed to parse definition: %w", err)
	}

	alias := raDef.Alias
	if alias == "" {
		alias = ra.Name
	}

	return kc.DeleteRequiredAction(ctx, realmName, alias)
}

func (r *KeycloakRequiredActionReconciler) getKeycloakClientAndRealm(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction) (*keycloak.Client, string, error) {
	if ra.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, ra.Spec.ClusterRealmRef.Name)
	}

	if ra.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := ra.Namespace
	if ra.Spec.RealmRef.Namespace != nil {
		realmNamespace = *ra.Spec.RealmRef.Namespace
	}
	realmKey := types.NamespacedName{
		Name:      ra.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmKey, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmKey, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmKey)
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	kc, err := GetKeycloakClientFromRealmInstance(ctx, r.Client, r.ClientManager, realm)
	if err != nil {
		return nil, "", err
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakRequiredActionReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	realmName := clusterRealm.Status.RealmName
	if realmName == "" {
		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(clusterRealm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", fmt.Errorf("failed to parse cluster realm definition: %w", err)
		}
		realmName = realmDef.Realm
	}

	if clusterRealm.Spec.ClusterInstanceRef != nil {
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, clusterInstance)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, nil
	}

	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceKey := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceKey, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceKey, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceKey)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceKey.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceKey)
	}

	return kc, realmName, nil
}

func (r *KeycloakRequiredActionReconciler) updateStatus(ctx context.Context, ra *keycloakv1beta1.KeycloakRequiredAction, ready bool, status, message, alias string) (ctrl.Result, error) {
	ra.Status.Ready = ready
	ra.Status.Status = status
	ra.Status.Message = message
	ra.Status.Alias = alias

	if ready {
		ra.Status.ObservedGeneration = ra.Generation
	}

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             status,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	if ready {
		condition.Status = metav1.ConditionTrue
	}

	found := false
	for i, c := range ra.Status.Conditions {
		if c.Type == "Ready" {
			ra.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		ra.Status.Conditions = append(ra.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, ra); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRequiredActionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRequiredAction{}).
		Complete(r)
}
