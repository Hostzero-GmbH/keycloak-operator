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

// KeycloakRoleReconciler reconciles a KeycloakRole object
type KeycloakRoleReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakroles/finalizers,verbs=update

// Reconcile handles KeycloakRole reconciliation
func (r *KeycloakRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakRole"

	// Fetch the KeycloakRole
	role := &keycloakv1beta1.KeycloakRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRole")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, role.Status.Ready, time.Since(startTime).Seconds())
	}()

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

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, role)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, role, false, "RealmNotReady", err.Error(), "", "", false, "")
	}

	// Parse role definition to extract name
	var roleDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(role.Spec.Definition.Raw, &roleDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, role, false, "InvalidDefinition", fmt.Sprintf("Failed to parse role definition: %v", err), "", "", false, "")
	}

	// Ensure name is set
	roleName := roleDef.Name
	if roleName == "" {
		roleName = role.Name
	}

	// Prepare definition with name set
	definition := setFieldInDefinition(role.Spec.Definition.Raw, "name", roleName)

	// Check if this is a client role
	var clientUUID string
	isClientRole := role.Spec.ClientRef != nil
	if isClientRole {
		clientUUID, err = r.getClientUUID(ctx, role)
		if err != nil {
			RecordError(controllerName, "client_not_ready")
			return r.updateStatus(ctx, role, false, "ClientNotReady", err.Error(), "", "", false, "")
		}
	}

	var roleID string
	if isClientRole {
		// Client role
		existingRole, err := kc.GetClientRole(ctx, realmName, clientUUID, roleName)
		if err != nil || existingRole == nil {
			// Create client role
			log.Info("creating client role", "name", roleName, "realm", realmName, "client", clientUUID)
			roleID, err = kc.CreateClientRole(ctx, realmName, clientUUID, definition)
			if err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "CreateFailed", fmt.Sprintf("Failed to create client role: %v", err), "", "", true, clientUUID)
			}
			log.Info("client role created successfully", "name", roleName, "id", roleID)
		} else {
			// Update client role
			roleID = *existingRole.ID
			definition = mergeIDIntoDefinition(definition, existingRole.ID)
			log.Info("updating client role", "name", roleName, "realm", realmName, "client", clientUUID)
			if err := kc.UpdateClientRole(ctx, realmName, clientUUID, roleName, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "UpdateFailed", fmt.Sprintf("Failed to update client role: %v", err), roleID, roleName, true, clientUUID)
			}
			log.Info("client role updated successfully", "name", roleName)
		}
	} else {
		// Realm role
		existingRole, err := kc.GetRealmRole(ctx, realmName, roleName)
		if err != nil || existingRole == nil {
			// Create realm role
			log.Info("creating realm role", "name", roleName, "realm", realmName)
			roleID, err = kc.CreateRealmRole(ctx, realmName, definition)
			if err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "CreateFailed", fmt.Sprintf("Failed to create realm role: %v", err), "", "", false, "")
			}
			log.Info("realm role created successfully", "name", roleName, "id", roleID)
		} else {
			// Update realm role
			roleID = *existingRole.ID
			definition = mergeIDIntoDefinition(definition, existingRole.ID)
			log.Info("updating realm role", "name", roleName, "realm", realmName)
			if err := kc.UpdateRealmRole(ctx, realmName, roleName, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "UpdateFailed", fmt.Sprintf("Failed to update realm role: %v", err), roleID, roleName, false, "")
			}
			log.Info("realm role updated successfully", "name", roleName)
		}
	}

	// Update status
	if isClientRole {
		role.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/clients/%s/roles/%s", realmName, clientUUID, roleName)
	} else {
		role.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/roles/%s", realmName, roleName)
	}
	return r.updateStatus(ctx, role, true, "Ready", "Role synchronized", roleID, roleName, isClientRole, clientUUID)
}

func (r *KeycloakRoleReconciler) getKeycloakClientAndRealm(ctx context.Context, role *keycloakv1beta1.KeycloakRole) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if role.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, role.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if role.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := role.Namespace
	if role.Spec.RealmRef.Namespace != nil {
		realmNamespace = *role.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      role.Spec.RealmRef.Name,
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

func (r *KeycloakRoleReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

func (r *KeycloakRoleReconciler) getClientUUID(ctx context.Context, role *keycloakv1beta1.KeycloakRole) (string, error) {
	if role.Spec.ClientRef == nil {
		return "", fmt.Errorf("clientRef is required for client roles")
	}

	clientNamespace := role.Namespace
	if role.Spec.ClientRef.Namespace != nil {
		clientNamespace = *role.Spec.ClientRef.Namespace
	}
	clientName := types.NamespacedName{
		Name:      role.Spec.ClientRef.Name,
		Namespace: clientNamespace,
	}

	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, clientName, kcClient); err != nil {
		return "", fmt.Errorf("failed to get KeycloakClient %s: %w", clientName, err)
	}

	if !kcClient.Status.Ready {
		return "", fmt.Errorf("KeycloakClient %s is not ready", clientName)
	}

	if kcClient.Status.ClientUUID == "" {
		return "", fmt.Errorf("KeycloakClient %s has no clientUUID", clientName)
	}

	return kcClient.Status.ClientUUID, nil
}

func (r *KeycloakRoleReconciler) deleteRole(ctx context.Context, role *keycloakv1beta1.KeycloakRole) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, role)
	if err != nil {
		return err
	}

	if role.Status.RoleName == "" {
		return nil // No role name stored, nothing to delete
	}

	if role.Status.IsClientRole && role.Status.ClientID != "" {
		return kc.DeleteClientRole(ctx, realmName, role.Status.ClientID, role.Status.RoleName)
	}
	return kc.DeleteRealmRole(ctx, realmName, role.Status.RoleName)
}

func (r *KeycloakRoleReconciler) updateStatus(ctx context.Context, role *keycloakv1beta1.KeycloakRole, ready bool, status, message, roleID, roleName string, isClientRole bool, clientID string) (ctrl.Result, error) {
	role.Status.Ready = ready
	role.Status.Status = status
	role.Status.Message = message
	role.Status.RoleID = roleID
	role.Status.RoleName = roleName
	role.Status.IsClientRole = isClientRole
	role.Status.ClientID = clientID

	if ready {
		role.Status.ObservedGeneration = role.Generation
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

	found := false
	for i, c := range role.Status.Conditions {
		if c.Type == "Ready" {
			role.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		role.Status.Conditions = append(role.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRole{}).
		Complete(r)
}
