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

// KeycloakClientScopeReconciler reconciles a KeycloakClientScope object
type KeycloakClientScopeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/finalizers,verbs=update

// Reconcile handles KeycloakClientScope reconciliation
func (r *KeycloakClientScopeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakClientScope
	scope := &keycloakv1beta1.KeycloakClientScope{}
	if err := r.Get(ctx, req.NamespacedName, scope); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakClientScope")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !scope.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(scope, FinalizerName) {
			if err := r.deleteClientScope(ctx, scope); err != nil {
				log.Error(err, "failed to delete client scope from Keycloak")
			}
			controllerutil.RemoveFinalizer(scope, FinalizerName)
			if err := r.Update(ctx, scope); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(scope, FinalizerName) {
		controllerutil.AddFinalizer(scope, FinalizerName)
		if err := r.Update(ctx, scope); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, scope)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, scope, false, "Error", err.Error())
	}

	// Parse scope definition
	var scopeDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(scope.Spec.Definition.Raw, &scopeDef); err != nil {
		log.Error(err, "failed to parse client scope definition")
		return r.updateStatus(ctx, scope, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Check if client scope exists
	existingScope, err := kc.GetClientScopeByName(ctx, realmName, scopeDef.Name)
	if err != nil {
		// Scope doesn't exist, create it
		scopeID, err := kc.CreateClientScope(ctx, realmName, scope.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create client scope")
			return r.updateStatus(ctx, scope, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created client scope", "name", scopeDef.Name, "id", scopeID)
		scope.Status.ScopeID = scopeID
		return r.updateStatus(ctx, scope, true, "Created", "Client scope created successfully")
	}

	// Scope exists, update it
	if err := kc.UpdateClientScope(ctx, realmName, *existingScope.ID, scope.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update client scope")
		return r.updateStatus(ctx, scope, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated client scope", "name", scopeDef.Name, "id", *existingScope.ID)
	scope.Status.ScopeID = *existingScope.ID
	return r.updateStatus(ctx, scope, true, "Ready", "Client scope synchronized")
}

func (r *KeycloakClientScopeReconciler) updateStatus(ctx context.Context, scope *keycloakv1beta1.KeycloakClientScope, ready bool, status, message string) (ctrl.Result, error) {
	scope.Status.Ready = ready
	scope.Status.Status = status
	scope.Status.Message = message

	if err := r.Status().Update(ctx, scope); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakClientScopeReconciler) getKeycloakClientAndRealm(ctx context.Context, scope *keycloakv1beta1.KeycloakClientScope) (*keycloak.Client, string, error) {
	// Get the referenced realm
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      scope.Spec.RealmRef.Name,
		Namespace: scope.Namespace,
	}
	if scope.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *scope.Spec.RealmRef.Namespace
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

func (r *KeycloakClientScopeReconciler) deleteClientScope(ctx context.Context, scope *keycloakv1beta1.KeycloakClientScope) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, scope)
	if err != nil {
		return err
	}

	var scopeDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(scope.Spec.Definition.Raw, &scopeDef); err != nil {
		return err
	}

	existingScope, err := kc.GetClientScopeByName(ctx, realmName, scopeDef.Name)
	if err != nil {
		return nil // Scope doesn't exist
	}

	return kc.DeleteClientScope(ctx, realmName, *existingScope.ID)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakClientScopeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClientScope{}).
		Complete(r)
}
