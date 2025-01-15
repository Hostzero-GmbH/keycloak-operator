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

// KeycloakRoleMappingReconciler reconciles a KeycloakRoleMapping object
type KeycloakRoleMappingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings/finalizers,verbs=update

// Reconcile handles KeycloakRoleMapping reconciliation
func (r *KeycloakRoleMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakRoleMapping
	mapping := &keycloakv1beta1.KeycloakRoleMapping{}
	if err := r.Get(ctx, req.NamespacedName, mapping); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRoleMapping")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !mapping.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mapping, FinalizerName) {
			if err := r.deleteRoleMappings(ctx, mapping); err != nil {
				log.Error(err, "failed to delete role mappings from Keycloak")
			}
			controllerutil.RemoveFinalizer(mapping, FinalizerName)
			if err := r.Update(ctx, mapping); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mapping, FinalizerName) {
		controllerutil.AddFinalizer(mapping, FinalizerName)
		if err := r.Update(ctx, mapping); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and user info
	kc, realmName, userID, err := r.getKeycloakClientAndUser(ctx, mapping)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, mapping, false, "Error", err.Error())
	}

	// Apply realm role mappings
	if len(mapping.Spec.RealmRoles) > 0 {
		roles := make([]keycloak.RoleRepresentation, 0, len(mapping.Spec.RealmRoles))
		for _, roleName := range mapping.Spec.RealmRoles {
			role, err := kc.GetRealmRole(ctx, realmName, roleName)
			if err != nil {
				log.Error(err, "failed to get realm role", "role", roleName)
				return r.updateStatus(ctx, mapping, false, "Error", fmt.Sprintf("Role not found: %s", roleName))
			}
			roles = append(roles, *role)
		}

		if err := kc.AddRealmRolesToUser(ctx, realmName, userID, roles); err != nil {
			log.Error(err, "failed to add realm roles to user")
			return r.updateStatus(ctx, mapping, false, "Error", fmt.Sprintf("Failed to add roles: %v", err))
		}
	}

	// Apply client role mappings
	for clientName, roleNames := range mapping.Spec.ClientRoles {
		client, err := kc.GetClientByClientID(ctx, realmName, clientName)
		if err != nil {
			log.Error(err, "failed to get client", "client", clientName)
			return r.updateStatus(ctx, mapping, false, "Error", fmt.Sprintf("Client not found: %s", clientName))
		}

		roles := make([]keycloak.RoleRepresentation, 0, len(roleNames))
		for _, roleName := range roleNames {
			role, err := kc.GetClientRole(ctx, realmName, *client.ID, roleName)
			if err != nil {
				log.Error(err, "failed to get client role", "client", clientName, "role", roleName)
				return r.updateStatus(ctx, mapping, false, "Error", fmt.Sprintf("Client role not found: %s/%s", clientName, roleName))
			}
			roles = append(roles, *role)
		}

		if err := kc.AddClientRolesToUser(ctx, realmName, userID, *client.ID, roles); err != nil {
			log.Error(err, "failed to add client roles to user")
			return r.updateStatus(ctx, mapping, false, "Error", fmt.Sprintf("Failed to add client roles: %v", err))
		}
	}

	log.Info("role mappings applied", "user", userID)
	return r.updateStatus(ctx, mapping, true, "Ready", "Role mappings applied")
}

func (r *KeycloakRoleMappingReconciler) updateStatus(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping, ready bool, status, message string) (ctrl.Result, error) {
	mapping.Status.Ready = ready
	mapping.Status.Status = status
	mapping.Status.Message = message

	if err := r.Status().Update(ctx, mapping); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakRoleMappingReconciler) getKeycloakClientAndUser(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) (*keycloak.Client, string, string, error) {
	// Get the referenced user
	user := &keycloakv1beta1.KeycloakUser{}
	userName := types.NamespacedName{
		Name:      mapping.Spec.UserRef.Name,
		Namespace: mapping.Namespace,
	}
	if mapping.Spec.UserRef.Namespace != nil {
		userName.Namespace = *mapping.Spec.UserRef.Namespace
	}
	if err := r.Get(ctx, userName, user); err != nil {
		return nil, "", "", fmt.Errorf("failed to get user: %w", err)
	}

	// Get the realm from the user
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: user.Namespace,
	}
	if user.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *user.Spec.RealmRef.Namespace
	}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get realm: %w", err)
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
		return nil, "", "", fmt.Errorf("failed to get instance: %w", err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", err
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get user ID
	if user.Status.UserID == "" {
		return nil, "", "", fmt.Errorf("user does not have an ID yet")
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, user.Status.UserID, nil
}

func (r *KeycloakRoleMappingReconciler) deleteRoleMappings(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) error {
	kc, realmName, userID, err := r.getKeycloakClientAndUser(ctx, mapping)
	if err != nil {
		return err
	}

	// Remove realm role mappings
	if len(mapping.Spec.RealmRoles) > 0 {
		roles := make([]keycloak.RoleRepresentation, 0, len(mapping.Spec.RealmRoles))
		for _, roleName := range mapping.Spec.RealmRoles {
			role, err := kc.GetRealmRole(ctx, realmName, roleName)
			if err != nil {
				continue
			}
			roles = append(roles, *role)
		}
		if len(roles) > 0 {
			_ = kc.DeleteRealmRolesFromUser(ctx, realmName, userID, roles)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRoleMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRoleMapping{}).
		Complete(r)
}
