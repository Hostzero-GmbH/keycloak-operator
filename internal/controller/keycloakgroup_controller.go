package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
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
			// Delete group from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(group) {
				log.Info("preserving group in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteGroup(ctx, group); err != nil {
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
		reason, metric := "RealmNotReady", "realm_not_ready"
		if group.Spec.ParentGroupRef != nil {
			reason, metric = "ParentNotReady", "parent_not_ready"
		}
		RecordError(controllerName, metric)
		return r.updateStatus(ctx, group, false, reason, err.Error(), "")
	}

	// Parse group definition to extract name
	var groupDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(group.Spec.Definition.Raw, &groupDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, group, false, "InvalidDefinition", fmt.Sprintf("Failed to parse group definition: %v", err), "")
	}

	// Resolve the group name from spec.name.
	groupName, err := resolveIdentifier("name", group.Spec.Name, groupDef.Name)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, group, false, InvalidIdentifierReason, err.Error(), "")
	}
	groupDef.Name = groupName
	group.Status.GroupName = groupName

	// Prepare definition JSON with name set
	definition := setFieldInDefinition(group.Spec.Definition.Raw, "name", groupName)

	// Check for parent group
	var parentGroupID string
	if group.Spec.ParentGroupRef != nil {
		parentGroup := &keycloakv1beta1.KeycloakGroup{}
		parentName := types.NamespacedName{
			Name:      group.Spec.ParentGroupRef.Name,
			Namespace: group.Namespace,
		}
		if err := r.Get(ctx, parentName, parentGroup); err != nil {
			return r.updateStatus(ctx, group, false, "ParentNotReady", fmt.Sprintf("Failed to get parent group: %v", err), "")
		}
		if !parentGroup.Status.Ready || parentGroup.Status.GroupID == "" {
			return r.updateStatus(ctx, group, false, "ParentNotReady", "Parent group is not ready", "")
		}
		parentGroupID = parentGroup.Status.GroupID
	}

	// Check if group exists by name. When a parent is set, scope the lookup to
	// the parent's children — Keycloak 23+ no longer inlines subGroups in the
	// realm-wide /groups response, so we cannot rely on a recursive walk.
	var existingGroup *keycloak.GroupRepresentation
	if parentGroupID != "" {
		children, err := kc.GetGroupChildren(ctx, realmName, parentGroupID, map[string]string{
			"search": groupDef.Name,
			"exact":  "true",
		})
		if err == nil {
			existingGroup = findTopLevelGroupByName(children, groupDef.Name)
		}
	} else {
		existingGroups, err := kc.GetGroups(ctx, realmName, map[string]string{
			"search": groupDef.Name,
			"exact":  "true",
		})
		if err == nil {
			existingGroup = findTopLevelGroupByName(existingGroups, groupDef.Name)
		}
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

// findTopLevelGroupByName returns the first group in the list whose name
// matches exactly. The list is expected to already be scoped to the right
// parent (either top-level or a specific parent's children); we deliberately
// do not recurse into SubGroups, which would be incorrect across parents and
// is empty anyway on Keycloak 23+.
func findTopLevelGroupByName(groups []keycloak.GroupRepresentation, name string) *keycloak.GroupRepresentation {
	for i := range groups {
		g := &groups[i]
		if g.Name != nil && *g.Name == name {
			return g
		}
	}
	return nil
}

// maxGroupNestingDepth caps the parentGroupRef walk. Keycloak imposes no limit on
// nesting; the cap only turns an accidental reference cycle into an error rather
// than an unbounded walk.
const maxGroupNestingDepth = 100

func (r *KeycloakGroupReconciler) getKeycloakClientAndRealm(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) (*keycloak.Client, string, error) {
	// A nested group names no realm of its own; it inherits the one carried by the
	// root of its parent chain.
	owner, err := resolveGroupRealmOwner(ctx, r.Client, group)
	if err != nil {
		return nil, "", err
	}

	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, owner.Namespace, owner.Spec.RealmRef, owner.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", err
	}
	return res.Client, res.RealmName, nil
}

// resolveGroupRealmOwner walks parentGroupRef upwards and returns the ancestor
// that carries the realm reference, which for a top-level group is the group
// itself. Shared by every controller that needs to resolve a group's realm.
func resolveGroupRealmOwner(ctx context.Context, c client.Client, group *keycloakv1beta1.KeycloakGroup) (*keycloakv1beta1.KeycloakGroup, error) {
	current := group
	seen := make(map[string]bool, 1)

	for range maxGroupNestingDepth {
		if current.Spec.ParentGroupRef == nil {
			if current.Spec.RealmRef == nil && current.Spec.ClusterRealmRef == nil {
				return nil, fmt.Errorf("KeycloakGroup %s/%s has no realmRef or clusterRealmRef", current.Namespace, current.Name)
			}
			return current, nil
		}

		key := current.Namespace + "/" + current.Name
		if seen[key] {
			return nil, fmt.Errorf("parentGroupRef cycle detected at KeycloakGroup %s", key)
		}
		seen[key] = true

		parentKey := types.NamespacedName{
			Name:      current.Spec.ParentGroupRef.Name,
			Namespace: current.Namespace,
		}
		parent := &keycloakv1beta1.KeycloakGroup{}
		if err := c.Get(ctx, parentKey, parent); err != nil {
			return nil, fmt.Errorf("failed to get parent KeycloakGroup %s: %w", parentKey, err)
		}
		current = parent
	}

	return nil, fmt.Errorf("parentGroupRef chain from KeycloakGroup %s/%s exceeds %d levels", group.Namespace, group.Name, maxGroupNestingDepth)
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

	group.Status.Conditions = setReadyCondition(group.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, group, ready)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakGroup{}).
		Complete(r)
}
