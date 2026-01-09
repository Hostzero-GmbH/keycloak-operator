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

// KeycloakGroupReconciler reconciles a KeycloakGroup object
type KeycloakGroupReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups/finalizers,verbs=update

// Reconcile handles KeycloakGroup reconciliation
func (r *KeycloakGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakGroup"

	// Fetch the KeycloakGroup
	group := &keycloakv1beta1.KeycloakGroup{}
	if err := r.Get(ctx, req.NamespacedName, group); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakGroup")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, group.Status.Ready, time.Since(startTime).Seconds())
	}()

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
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, group, false, "RealmNotReady", err.Error(), "")
	}

	// Parse group definition to extract name
	var groupDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(group.Spec.Definition.Raw, &groupDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, group, false, "InvalidDefinition", fmt.Sprintf("Failed to parse group definition: %v", err), "")
	}

	// Ensure name is set
	if groupDef.Name == "" {
		// Default to metadata.name
		groupDef.Name = group.Name
	}

	// Prepare definition JSON with name set
	definition := setFieldInDefinition(group.Spec.Definition.Raw, "name", groupDef.Name)

	// Check for parent group
	var parentGroupID string
	if group.Spec.ParentGroupRef != nil {
		parentGroup := &keycloakv1beta1.KeycloakGroup{}
		parentNamespace := group.Namespace
		if group.Spec.ParentGroupRef.Namespace != nil {
			parentNamespace = *group.Spec.ParentGroupRef.Namespace
		}
		parentName := types.NamespacedName{
			Name:      group.Spec.ParentGroupRef.Name,
			Namespace: parentNamespace,
		}
		if err := r.Get(ctx, parentName, parentGroup); err != nil {
			return r.updateStatus(ctx, group, false, "ParentNotReady", fmt.Sprintf("Failed to get parent group: %v", err), "")
		}
		if !parentGroup.Status.Ready || parentGroup.Status.GroupID == "" {
			return r.updateStatus(ctx, group, false, "ParentNotReady", "Parent group is not ready", "")
		}
		parentGroupID = parentGroup.Status.GroupID
	}

	// Check if group exists by name
	existingGroups, err := kc.GetGroups(ctx, realmName, map[string]string{
		"search": groupDef.Name,
	})
	var existingGroup *keycloak.GroupRepresentation
	if err == nil {
		// Search through groups (including nested) for exact name match
		existingGroup = findGroupByNameInList(existingGroups, groupDef.Name, parentGroupID)
	}

	var groupID string
	if existingGroup == nil {
		// Group doesn't exist, create it
		log.Info("creating group", "name", groupDef.Name, "realm", realmName)

		if parentGroupID != "" {
			// Create as child group
			groupID, err = kc.CreateChildGroup(ctx, realmName, parentGroupID, definition)
		} else {
			// Create as top-level group
			groupID, err = kc.CreateGroup(ctx, realmName, definition)
		}

		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, group, false, "CreateFailed", fmt.Sprintf("Failed to create group: %v", err), "")
		}
		log.Info("group created successfully", "name", groupDef.Name, "id", groupID)
	} else {
		// Group exists, update it
		groupID = *existingGroup.ID
		definition = mergeIDIntoDefinition(definition, existingGroup.ID)

		log.Info("updating group", "name", groupDef.Name, "realm", realmName)
		if err := kc.UpdateGroup(ctx, realmName, groupID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, group, false, "UpdateFailed", fmt.Sprintf("Failed to update group: %v", err), groupID)
		}
		log.Info("group updated successfully", "name", groupDef.Name)
	}

	// Update status
	group.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/groups/%s", realmName, groupID)
	return r.updateStatus(ctx, group, true, "Ready", "Group synchronized", groupID)
}

// findGroupByNameInList searches for a group by name in a list
func findGroupByNameInList(groups []keycloak.GroupRepresentation, name string, parentID string) *keycloak.GroupRepresentation {
	for i := range groups {
		g := &groups[i]
		if g.Name != nil && *g.Name == name {
			// If no parent specified, return top-level match
			if parentID == "" {
				return g
			}
			// With parent, check if group is within that parent's subgroups
			// Note: We've already searched within the parent context
			return g
		}
		// Search subgroups recursively
		if len(g.SubGroups) > 0 {
			if found := findGroupByNameInList(g.SubGroups, name, parentID); found != nil {
				return found
			}
		}
	}
	return nil
}

func (r *KeycloakGroupReconciler) getKeycloakClientAndRealm(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if group.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, group.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if group.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := group.Namespace
	if group.Spec.RealmRef.Namespace != nil {
		realmNamespace = *group.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      group.Spec.RealmRef.Name,
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

func (r *KeycloakGroupReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

func (r *KeycloakGroupReconciler) deleteGroup(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, group)
	if err != nil {
		return err
	}

	if group.Status.GroupID == "" {
		return nil // No group ID stored, nothing to delete
	}

	return kc.DeleteGroup(ctx, realmName, group.Status.GroupID)
}

func (r *KeycloakGroupReconciler) updateStatus(ctx context.Context, group *keycloakv1beta1.KeycloakGroup, ready bool, status, message, groupID string) (ctrl.Result, error) {
	group.Status.Ready = ready
	group.Status.Status = status
	group.Status.Message = message
	if groupID != "" {
		group.Status.GroupID = groupID
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
	for i, c := range group.Status.Conditions {
		if c.Type == "Ready" {
			group.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		group.Status.Conditions = append(group.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, group); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakGroup{}).
		Complete(r)
}
