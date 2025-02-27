package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakComponentReconciler reconciles a KeycloakComponent object
type KeycloakComponentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents/finalizers,verbs=update

// Reconcile handles KeycloakComponent reconciliation
func (r *KeycloakComponentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	component := &keycloakv1beta1.KeycloakComponent{}
	if err := r.Get(ctx, req.NamespacedName, component); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakComponent")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !component.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(component, FinalizerName) {
			if err := r.deleteComponent(ctx, component); err != nil {
				log.Error(err, "failed to delete component from Keycloak")
			}
			controllerutil.RemoveFinalizer(component, FinalizerName)
			if err := r.Update(ctx, component); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(component, FinalizerName) {
		controllerutil.AddFinalizer(component, FinalizerName)
		if err := r.Update(ctx, component); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, component)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, component, false, "Error", err.Error())
	}

	// Parse component definition
	var componentDef struct {
		Name         string `json:"name"`
		ProviderType string `json:"providerType"`
	}
	if err := json.Unmarshal(component.Spec.Definition.Raw, &componentDef); err != nil {
		log.Error(err, "failed to parse component definition")
		return r.updateStatus(ctx, component, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Create or update component
	if component.Status.ComponentID == "" {
		componentID, err := kc.CreateComponent(ctx, realmName, component.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create component")
			return r.updateStatus(ctx, component, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created component", "name", componentDef.Name, "id", componentID)
		component.Status.ComponentID = componentID
		return r.updateStatus(ctx, component, true, "Created", "Component created")
	}

	// Update
	if err := kc.UpdateComponent(ctx, realmName, component.Status.ComponentID, component.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update component")
		return r.updateStatus(ctx, component, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated component", "name", componentDef.Name)
	return r.updateStatus(ctx, component, true, "Ready", "Component synchronized")
}

func (r *KeycloakComponentReconciler) updateStatus(ctx context.Context, component *keycloakv1beta1.KeycloakComponent, ready bool, status, message string) (ctrl.Result, error) {
	component.Status.Ready = ready
	component.Status.Status = status
	component.Status.Message = message

	if err := r.Status().Update(ctx, component); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakComponentReconciler) getKeycloakClientAndRealm(ctx context.Context, component *keycloakv1beta1.KeycloakComponent) (*keycloak.Client, string, error) {
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      component.Spec.RealmRef.Name,
		Namespace: component.Namespace,
	}
	if component.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *component.Spec.RealmRef.Namespace
	}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get realm: %w", err)
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: realm.Namespace,
	}
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *realm.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get instance: %w", err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", err
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, nil
}

func (r *KeycloakComponentReconciler) deleteComponent(ctx context.Context, component *keycloakv1beta1.KeycloakComponent) error {
	if component.Status.ComponentID == "" {
		return nil
	}

	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, component)
	if err != nil {
		return err
	}

	return kc.DeleteComponent(ctx, realmName, component.Status.ComponentID)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakComponent{}).
		Complete(r)
}
