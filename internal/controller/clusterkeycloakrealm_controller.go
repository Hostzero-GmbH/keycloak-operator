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

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// ClusterKeycloakRealmReconciler reconciles a ClusterKeycloakRealm object
type ClusterKeycloakRealmReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakinstances,verbs=get;list;watch

// Reconcile handles ClusterKeycloakRealm reconciliation
func (r *ClusterKeycloakRealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "ClusterKeycloakRealm"

	// Fetch the ClusterKeycloakRealm
	realm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, req.NamespacedName, realm); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch ClusterKeycloakRealm")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, realm.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !realm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(realm, FinalizerName) {
			// Delete realm from Keycloak
			if err := r.deleteRealm(ctx, realm); err != nil {
				log.Error(err, "failed to delete realm from Keycloak")
				// Continue with finalizer removal even on error
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(realm, FinalizerName)
			if err := r.Update(ctx, realm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(realm, FinalizerName) {
		controllerutil.AddFinalizer(realm, FinalizerName)
		if err := r.Update(ctx, realm); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client for this realm's instance
	kc, instanceRef, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		RecordError(controllerName, "instance_not_ready")
		return r.updateStatus(ctx, realm, false, "InstanceNotReady", err.Error(), instanceRef)
	}

	// Parse realm definition to extract realm name
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", fmt.Sprintf("Failed to parse realm definition: %v", err), instanceRef)
	}

	// Ensure realm name is set
	if realmDef.Realm == "" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", "Realm name is required in definition", instanceRef)
	}

	// Check if realm exists
	existingRealm, err := kc.GetRealm(ctx, realmDef.Realm)
	if err != nil {
		// Realm doesn't exist, create it
		log.Info("creating realm", "realm", realmDef.Realm)
		if err := kc.CreateRealmFromDefinition(ctx, realm.Spec.Definition.Raw); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, realm, false, "CreateFailed", fmt.Sprintf("Failed to create realm: %v", err), instanceRef)
		}
		log.Info("realm created successfully", "realm", realmDef.Realm)
	} else {
		// Realm exists, update it - merge ID into definition
		log.Info("updating realm", "realm", realmDef.Realm)
		definition := mergeIDIntoDefinition(realm.Spec.Definition.Raw, existingRealm.ID)
		if err := kc.UpdateRealm(ctx, realmDef.Realm, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, realm, false, "UpdateFailed", fmt.Sprintf("Failed to update realm: %v", err), instanceRef)
		}
		log.Info("realm updated successfully", "realm", realmDef.Realm)
	}

	// Update status
	realm.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s", realmDef.Realm)
	realm.Status.RealmName = realmDef.Realm
	return r.updateStatus(ctx, realm, true, "Ready", "Realm synchronized", instanceRef)
}

func (r *ClusterKeycloakRealmReconciler) getKeycloakClient(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm) (*keycloak.Client, *keycloakv1beta1.InstanceRef, error) {
	// Determine if we're using cluster or namespaced instance
	if realm.Spec.ClusterInstanceRef != nil {
		// Using ClusterKeycloakInstance
		instanceRef := &keycloakv1beta1.InstanceRef{
			ClusterInstanceRef: realm.Spec.ClusterInstanceRef.Name,
		}

		instance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: realm.Spec.ClusterInstanceRef.Name}, instance); err != nil {
			return nil, instanceRef, fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", realm.Spec.ClusterInstanceRef.Name, err)
		}

		if !instance.Status.Ready {
			return nil, instanceRef, fmt.Errorf("ClusterKeycloakInstance %s is not ready", realm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, instance)
		if err != nil {
			return nil, instanceRef, fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", realm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(realm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, instanceRef, fmt.Errorf("keycloak client not available for cluster instance %s", realm.Spec.ClusterInstanceRef.Name)
		}

		return kc, instanceRef, nil
	}

	if realm.Spec.InstanceRef != nil {
		// Using namespaced KeycloakInstance
		instanceName := types.NamespacedName{
			Name:      realm.Spec.InstanceRef.Name,
			Namespace: realm.Spec.InstanceRef.Namespace,
		}

		instanceRef := &keycloakv1beta1.InstanceRef{
			InstanceRef: fmt.Sprintf("%s/%s", instanceName.Namespace, instanceName.Name),
		}

		instance := &keycloakv1beta1.KeycloakInstance{}
		if err := r.Get(ctx, instanceName, instance); err != nil {
			return nil, instanceRef, fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
		}

		if !instance.Status.Ready {
			return nil, instanceRef, fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
		}

		cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
		if err != nil {
			return nil, instanceRef, fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
		}

		kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
		if kc == nil {
			return nil, instanceRef, fmt.Errorf("keycloak client not available for instance %s", instanceName)
		}

		return kc, instanceRef, nil
	}

	return nil, nil, fmt.Errorf("either instanceRef or clusterInstanceRef must be specified")
}

func (r *ClusterKeycloakRealmReconciler) deleteRealm(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm) error {
	kc, _, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		return err
	}

	// Parse realm definition to get realm name
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return fmt.Errorf("failed to parse realm definition: %w", err)
	}

	if realmDef.Realm == "" {
		return fmt.Errorf("realm name not found in definition")
	}

	return kc.DeleteRealm(ctx, realmDef.Realm)
}

func (r *ClusterKeycloakRealmReconciler) updateStatus(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm, ready bool, status, message string, instanceRef *keycloakv1beta1.InstanceRef) (ctrl.Result, error) {
	realm.Status.Ready = ready
	realm.Status.Status = status
	realm.Status.Message = message
	realm.Status.Instance = instanceRef

	// Update conditions
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

	// Update or add condition
	found := false
	for i, c := range realm.Status.Conditions {
		if c.Type == "Ready" {
			realm.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		realm.Status.Conditions = append(realm.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, realm); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterKeycloakRealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.ClusterKeycloakRealm{}).
		Complete(r)
}
