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

// KeycloakClientScopeReconciler reconciles a KeycloakClientScope object
type KeycloakClientScopeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/finalizers,verbs=update

// Reconcile handles KeycloakClientScope reconciliation
func (r *KeycloakClientScopeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakClientScope"

	// Fetch the KeycloakClientScope
	clientScope := &keycloakv1beta1.KeycloakClientScope{}
	if err := r.Get(ctx, req.NamespacedName, clientScope); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakClientScope")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, clientScope.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !clientScope.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(clientScope, FinalizerName) {
			if err := r.deleteClientScope(ctx, clientScope); err != nil {
				log.Error(err, "failed to delete client scope from Keycloak")
			}

			controllerutil.RemoveFinalizer(clientScope, FinalizerName)
			if err := r.Update(ctx, clientScope); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(clientScope, FinalizerName) {
		controllerutil.AddFinalizer(clientScope, FinalizerName)
		if err := r.Update(ctx, clientScope); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, clientScope)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, clientScope, false, "RealmNotReady", err.Error(), "")
	}

	// Parse client scope definition to extract name
	var scopeDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(clientScope.Spec.Definition.Raw, &scopeDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, clientScope, false, "InvalidDefinition", fmt.Sprintf("Failed to parse client scope definition: %v", err), "")
	}

	// Ensure name is set
	if scopeDef.Name == "" {
		// Default to metadata.name
		scopeDef.Name = clientScope.Name
	}

	// Prepare definition JSON with name set
	definition := setFieldInDefinition(clientScope.Spec.Definition.Raw, "name", scopeDef.Name)

	// Check if client scope exists by name
	existingScopes, err := kc.GetClientScopes(ctx, realmName)
	var existingScope *keycloak.ClientScopeRepresentation
	if err == nil {
		for i := range existingScopes {
			if existingScopes[i].Name != nil && *existingScopes[i].Name == scopeDef.Name {
				existingScope = &existingScopes[i]
				break
			}
		}
	}

	var scopeID string
	if existingScope == nil {
		// Client scope doesn't exist, create it
		log.Info("creating client scope", "name", scopeDef.Name, "realm", realmName)
		scopeID, err = kc.CreateClientScope(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, clientScope, false, "CreateFailed", fmt.Sprintf("Failed to create client scope: %v", err), "")
		}
		log.Info("client scope created successfully", "name", scopeDef.Name, "id", scopeID)
	} else {
		// Client scope exists, update it
		scopeID = *existingScope.ID
		definition = mergeIDIntoDefinition(definition, existingScope.ID)

		log.Info("updating client scope", "name", scopeDef.Name, "realm", realmName)
		if err := kc.UpdateClientScope(ctx, realmName, scopeID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, clientScope, false, "UpdateFailed", fmt.Sprintf("Failed to update client scope: %v", err), scopeID)
		}
		log.Info("client scope updated successfully", "name", scopeDef.Name)
	}

	// Update status
	clientScope.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/client-scopes/%s", realmName, scopeID)
	return r.updateStatus(ctx, clientScope, true, "Ready", "Client scope synchronized", scopeID)
}

func (r *KeycloakClientScopeReconciler) getKeycloakClientAndRealm(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if clientScope.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, clientScope.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if clientScope.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := clientScope.Namespace
	if clientScope.Spec.RealmRef.Namespace != nil {
		realmNamespace = *clientScope.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      clientScope.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	// Get the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get instance reference from realm
	if realm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("realm %s has no instanceRef", realmName)
	}

	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakClientScopeReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
	// Get the ClusterKeycloakRealm
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	// Get realm name
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

	// Get Keycloak client from cluster instance
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
			return nil, "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, nil
	}

	// Use namespaced instance ref
	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceName := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, nil
}

func (r *KeycloakClientScopeReconciler) deleteClientScope(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, clientScope)
	if err != nil {
		return err
	}

	// Get scope ID from resource path or find by name
	var scopeDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(clientScope.Spec.Definition.Raw, &scopeDef); err != nil {
		return fmt.Errorf("failed to parse client scope definition: %w", err)
	}

	if scopeDef.Name == "" {
		scopeDef.Name = clientScope.Name
	}

	// Find scope by name
	scopes, err := kc.GetClientScopes(ctx, realmName)
	if err != nil {
		return err
	}

	for _, s := range scopes {
		if s.Name != nil && *s.Name == scopeDef.Name {
			return kc.DeleteClientScope(ctx, realmName, *s.ID)
		}
	}

	return nil // Scope doesn't exist
}

func (r *KeycloakClientScopeReconciler) updateStatus(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope, ready bool, status, message, scopeID string) (ctrl.Result, error) {
	clientScope.Status.Ready = ready
	clientScope.Status.Status = status
	clientScope.Status.Message = message

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

	found := false
	for i, c := range clientScope.Status.Conditions {
		if c.Type == "Ready" {
			clientScope.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		clientScope.Status.Conditions = append(clientScope.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, clientScope); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakClientScopeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClientScope{}).
		Complete(r)
}
