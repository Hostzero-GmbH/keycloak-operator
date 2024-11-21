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

// KeycloakRoleReconciler reconciles a KeycloakRole object
type KeycloakRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles/finalizers,verbs=update

// Reconcile handles KeycloakRole reconciliation
func (r *KeycloakRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakRole
	role := &keycloakv1beta1.KeycloakRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRole")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !role.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(role, FinalizerName) {
			if err := r.deleteRole(ctx, role); err != nil {
				log.Error(err, "failed to delete role from Keycloak")
			}
			controllerutil.RemoveFinalizer(role, FinalizerName)
			if err := r.Update(ctx, role); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(role, FinalizerName) {
		controllerutil.AddFinalizer(role, FinalizerName)
		if err := r.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client from instance
	kc, err := r.getKeycloakClient(ctx, role)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, role, false, "Error", err.Error())
	}

	// Parse role definition
	var roleDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(role.Spec.Definition.Raw, &roleDef); err != nil {
		log.Error(err, "failed to parse role definition")
		return r.updateStatus(ctx, role, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Get the realm name from instance - THIS IS WRONG, instance doesn't have realm info
	// for the target realm, only the admin realm
	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      role.Spec.InstanceRef.Name,
		Namespace: role.Namespace,
	}
	if role.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *role.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return r.updateStatus(ctx, role, false, "Error", fmt.Sprintf("Instance not found: %v", err))
	}

	// Use master realm as default - this is the conceptual mistake
	// roles should be created in a specific realm, not master
	realmName := "master"
	if instance.Spec.Realm != nil {
		realmName = *instance.Spec.Realm
	}

	// Check if role exists
	existingRole, err := kc.GetRealmRole(ctx, realmName, roleDef.Name)
	if err != nil {
		// Role doesn't exist, create it
		if err := kc.CreateRealmRole(ctx, realmName, role.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to create role")
			return r.updateStatus(ctx, role, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created role", "name", roleDef.Name)
		return r.updateStatus(ctx, role, true, "Created", "Role created successfully")
	}

	// Role exists, update it
	if err := kc.UpdateRealmRole(ctx, realmName, roleDef.Name, role.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update role")
		return r.updateStatus(ctx, role, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated role", "name", roleDef.Name, "id", *existingRole.ID)
	return r.updateStatus(ctx, role, true, "Ready", "Role synchronized")
}

func (r *KeycloakRoleReconciler) updateStatus(ctx context.Context, role *keycloakv1beta1.KeycloakRole, ready bool, status, message string) (ctrl.Result, error) {
	role.Status.Ready = ready
	role.Status.Status = status
	role.Status.Message = message

	if err := r.Status().Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakRoleReconciler) getKeycloakClient(ctx context.Context, role *keycloakv1beta1.KeycloakRole) (*keycloak.Client, error) {
	// Get the referenced instance
	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      role.Spec.InstanceRef.Name,
		Namespace: role.Namespace,
	}
	if role.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *role.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, err
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), nil
}

func (r *KeycloakRoleReconciler) deleteRole(ctx context.Context, role *keycloakv1beta1.KeycloakRole) error {
	kc, err := r.getKeycloakClient(ctx, role)
	if err != nil {
		return err
	}

	var roleDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(role.Spec.Definition.Raw, &roleDef); err != nil {
		return err
	}

	// Same mistake: using instance realm instead of target realm
	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      role.Spec.InstanceRef.Name,
		Namespace: role.Namespace,
	}
	if role.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *role.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return err
	}

	realmName := "master"
	if instance.Spec.Realm != nil {
		realmName = *instance.Spec.Realm
	}

	return kc.DeleteRealmRole(ctx, realmName, roleDef.Name)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRole{}).
		Complete(r)
}
