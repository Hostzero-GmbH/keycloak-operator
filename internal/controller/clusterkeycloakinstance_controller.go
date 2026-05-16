package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// ClusterKeycloakInstanceReconciler reconciles a ClusterKeycloakInstance object
type ClusterKeycloakInstanceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakinstances/finalizers,verbs=update

// Reconcile handles ClusterKeycloakInstance reconciliation
func (r *ClusterKeycloakInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "ClusterKeycloakInstance"

	// Fetch the ClusterKeycloakInstance
	instance := &keycloakv1beta1.ClusterKeycloakInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, remove client from manager
			r.ClientManager.RemoveClient(clusterInstanceKey(req.Name))
			SetKeycloakConnectionStatus(req.Name, "_cluster", false)
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch ClusterKeycloakInstance")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, instance.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, FinalizerName) {
			// Cleanup logic
			r.ClientManager.RemoveClient(clusterInstanceKey(req.Name))

			// Remove finalizer
			controllerutil.RemoveFinalizer(instance, FinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, FinalizerName) {
		controllerutil.AddFinalizer(instance, FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, instance)
	if err != nil {
		return r.updateStatus(ctx, instance, false, "", "Error", err.Error())
	}

	// Create/get Keycloak client
	kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(req.Name), cfg)

	// Ping Keycloak to verify connection
	if err := kc.Ping(ctx); err != nil {
		log.Error(err, "failed to connect to Keycloak")
		SetKeycloakConnectionStatus(instance.Name, "_cluster", false)
		RecordError(controllerName, "connection_failed")
		return r.updateStatus(ctx, instance, false, "", "ConnectionFailed", fmt.Sprintf("Failed to connect: %v", err))
	}

	// Get server version
	version := ""
	serverInfo, err := kc.GetServerInfo(ctx)
	if err != nil {
		log.Error(err, "failed to get server info")
	} else if serverInfo != nil && serverInfo.SystemInfo.Version != "" {
		version = serverInfo.SystemInfo.Version
	}

	// Update connection status metric
	SetKeycloakConnectionStatus(instance.Name, "_cluster", true)

	log.Info("successfully connected to Keycloak", "baseUrl", instance.Spec.BaseUrl, "version", version)
	return r.updateStatus(ctx, instance, true, version, "Ready", "Connected to Keycloak")
}

// clusterInstanceKey returns a unique key for cluster-scoped instances
func clusterInstanceKey(name string) string {
	return fmt.Sprintf("_cluster/%s", name)
}

func (r *ClusterKeycloakInstanceReconciler) updateStatus(ctx context.Context, instance *keycloakv1beta1.ClusterKeycloakInstance, ready bool, version, status, message string) (ctrl.Result, error) {
	instance.Status.Ready = ready
	instance.Status.Status = status
	instance.Status.Message = message
	if version != "" {
		instance.Status.Version = version
	}

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
	for i, c := range instance.Status.Conditions {
		if c.Type == "Ready" {
			instance.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		instance.Status.Conditions = append(instance.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterKeycloakInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.ClusterKeycloakInstance{}).
		Complete(r)
}
