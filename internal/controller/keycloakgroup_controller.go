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

// KeycloakGroupReconciler reconciles a KeycloakGroup object
type KeycloakGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups/finalizers,verbs=update

// Reconcile handles KeycloakGroup reconciliation
func (r *KeycloakGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakGroup
	group := &keycloakv1beta1.KeycloakGroup{}
	if err := r.Get(ctx, req.NamespacedName, group); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakGroup")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !group.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(group, FinalizerName) {
			if err := r.deleteGroup(ctx, group); err != nil {
				log.Error(err, "failed to delete group from Keycloak")
			}
			controllerutil.RemoveFinalizer(group, FinalizerName)
			if err := r.Update(ctx, group); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(group, FinalizerName) {
		controllerutil.AddFinalizer(group, FinalizerName)
		if err := r.Update(ctx, group); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, group)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, group, false, "Error", err.Error())
	}

	// Parse group definition
	var groupDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(group.Spec.Definition.Raw, &groupDef); err != nil {
		log.Error(err, "failed to parse group definition")
		return r.updateStatus(ctx, group, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Check if group exists
	existingGroup, err := kc.GetGroupByName(ctx, realmName, groupDef.Name)
	if err != nil {
		// Group doesn't exist, create it
		groupID, err := kc.CreateGroup(ctx, realmName, group.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create group")
			return r.updateStatus(ctx, group, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created group", "name", groupDef.Name, "id", groupID)
		group.Status.GroupID = groupID
		return r.updateStatus(ctx, group, true, "Created", "Group created successfully")
	}

	// Group exists, update it
	if err := kc.UpdateGroup(ctx, realmName, *existingGroup.ID, group.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update group")
		return r.updateStatus(ctx, group, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated group", "name", groupDef.Name, "id", *existingGroup.ID)
	group.Status.GroupID = *existingGroup.ID
	return r.updateStatus(ctx, group, true, "Ready", "Group synchronized")
}

func (r *KeycloakGroupReconciler) updateStatus(ctx context.Context, group *keycloakv1beta1.KeycloakGroup, ready bool, status, message string) (ctrl.Result, error) {
	group.Status.Ready = ready
	group.Status.Status = status
	group.Status.Message = message

	if err := r.Status().Update(ctx, group); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakGroupReconciler) getKeycloakClientAndRealm(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) (*keycloak.Client, string, error) {
	// Get the referenced realm
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      group.Spec.RealmRef.Name,
		Namespace: group.Namespace,
	}
	if group.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *group.Spec.RealmRef.Namespace
	}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get realm: %w", err)
	}

	// Get the instance from the realm
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

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, nil
}

func (r *KeycloakGroupReconciler) deleteGroup(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, group)
	if err != nil {
		return err
	}

	var groupDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(group.Spec.Definition.Raw, &groupDef); err != nil {
		return err
	}

	existingGroup, err := kc.GetGroupByName(ctx, realmName, groupDef.Name)
	if err != nil {
		return nil // Group doesn't exist
	}

	return kc.DeleteGroup(ctx, realmName, *existingGroup.ID)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakGroup{}).
		Complete(r)
}
