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
			// Delete role from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(role) {
				log.Info("preserving role in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteRole(ctx, role); err != nil {
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

	// Get Keycloak client, realm, and (for client roles) the owning client UUID
	kc, realmName, clientUUID, err := r.getKeycloakClientAndRealm(ctx, role)
	if err != nil {
		reason, metric := "RealmNotReady", "realm_not_ready"
		if role.Spec.ClientRef != nil {
			reason, metric = "ClientNotReady", "client_not_ready"
		}
		RecordError(controllerName, metric)
		return r.updateStatus(ctx, role, false, reason, err.Error(), "", "", false, "")
	}

	// Parse role definition to extract name
	var roleDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(role.Spec.Definition.Raw, &roleDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, role, false, "InvalidDefinition", fmt.Sprintf("Failed to parse role definition: %v", err), "", "", false, "")
	}

	// Resolve the role name from spec.name.
	roleName, err := resolveIdentifier("name", role.Spec.Name, roleDef.Name)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, role, false, InvalidIdentifierReason, err.Error(), "", "", false, "")
	}

	definition := setFieldInDefinition(role.Spec.Definition.Raw, "name", roleName)

	// Composites are not honored by the role create/update endpoints; manage
	// them via the dedicated composites endpoint after the role exists.
	desiredComposites, compositesRequested := extractRoleComposites(definition)
	definition = removeFieldFromDefinition(definition, "composites")

	isClientRole := role.Spec.ClientRef != nil

	var roleID string
	if isClientRole {
		existingRole, err := kc.GetClientRole(ctx, realmName, clientUUID, roleName)
		if err != nil || existingRole == nil {
			log.Info("creating client role", "name", roleName, "realm", realmName, "client", clientUUID)
			roleID, err = kc.CreateClientRole(ctx, realmName, clientUUID, definition)
			if err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "CreateFailed", fmt.Sprintf("Failed to create client role: %v", err), "", "", true, clientUUID)
			}
			log.Info("client role created successfully", "name", roleName, "id", roleID)
		} else {
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
		existingRole, err := kc.GetRealmRole(ctx, realmName, roleName)
		if err != nil || existingRole == nil {
			log.Info("creating realm role", "name", roleName, "realm", realmName)
			roleID, err = kc.CreateRealmRole(ctx, realmName, definition)
			if err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, role, false, "CreateFailed", fmt.Sprintf("Failed to create realm role: %v", err), "", "", false, "")
			}
			log.Info("realm role created successfully", "name", roleName, "id", roleID)
		} else {
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

	if compositesRequested {
		if err := r.syncRoleComposites(ctx, kc, realmName, roleName, isClientRole, clientUUID, desiredComposites); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, role, false, "CompositesFailed", fmt.Sprintf("Failed to sync role composites: %v", err), roleID, roleName, isClientRole, clientUUID)
		}
	}

	if isClientRole {
		role.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/clients/%s/roles/%s", realmName, clientUUID, roleName)
	} else {
		role.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/roles/%s", realmName, roleName)
	}
	return r.updateStatus(ctx, role, true, "Ready", "Role synchronized", roleID, roleName, isClientRole, clientUUID)
}

// syncRoleComposites diffs desired vs. existing composite members and applies
// add/remove via the dedicated composites endpoints.
func (r *KeycloakRoleReconciler) syncRoleComposites(
	ctx context.Context,
	kc *keycloak.Client,
	realmName, roleName string,
	isClientRole bool,
	clientUUID string,
	desired roleCompositesSpec,
) error {
	log := log.FromContext(ctx)

	desiredRoles, err := resolveRoleComposites(ctx, kc, realmName, desired)
	if err != nil {
		return err
	}

	var existing []keycloak.RoleRepresentation
	if isClientRole {
		existing, err = kc.GetClientRoleComposites(ctx, realmName, clientUUID, roleName)
	} else {
		existing, err = kc.GetRealmRoleComposites(ctx, realmName, roleName)
	}
	if err != nil {
		return fmt.Errorf("failed to list existing composites: %w", err)
	}

	desiredIDs := make(map[string]keycloak.RoleRepresentation, len(desiredRoles))
	for _, rr := range desiredRoles {
		if rr.ID != nil && *rr.ID != "" {
			desiredIDs[*rr.ID] = rr
		}
	}
	existingIDs := make(map[string]keycloak.RoleRepresentation, len(existing))
	for _, rr := range existing {
		if rr.ID != nil && *rr.ID != "" {
			existingIDs[*rr.ID] = rr
		}
	}

	var toAdd, toRemove []keycloak.RoleRepresentation
	for id, rr := range desiredIDs {
		if _, ok := existingIDs[id]; !ok {
			toAdd = append(toAdd, rr)
		}
	}
	for id, rr := range existingIDs {
		if _, ok := desiredIDs[id]; !ok {
			toRemove = append(toRemove, rr)
		}
	}

	if len(toAdd) > 0 {
		log.Info("adding composite role members", "role", roleName, "count", len(toAdd))
		if isClientRole {
			if err := kc.AddClientRoleComposites(ctx, realmName, clientUUID, roleName, toAdd); err != nil {
				return fmt.Errorf("failed to add composites: %w", err)
			}
		} else {
			if err := kc.AddRealmRoleComposites(ctx, realmName, roleName, toAdd); err != nil {
				return fmt.Errorf("failed to add composites: %w", err)
			}
		}
	}
	if len(toRemove) > 0 {
		log.Info("removing composite role members", "role", roleName, "count", len(toRemove))
		if isClientRole {
			if err := kc.RemoveClientRoleComposites(ctx, realmName, clientUUID, roleName, toRemove); err != nil {
				return fmt.Errorf("failed to remove composites: %w", err)
			}
		} else {
			if err := kc.RemoveRealmRoleComposites(ctx, realmName, roleName, toRemove); err != nil {
				return fmt.Errorf("failed to remove composites: %w", err)
			}
		}
	}
	return nil
}

// resolveRoleComposites looks up the Keycloak RoleRepresentations (with IDs)
// for the realm and client roles named in a composites spec.
func resolveRoleComposites(
	ctx context.Context,
	kc *keycloak.Client,
	realmName string,
	desired roleCompositesSpec,
) ([]keycloak.RoleRepresentation, error) {
	resolved := make([]keycloak.RoleRepresentation, 0, len(desired.Realm))
	for _, name := range desired.Realm {
		if name == "" {
			continue
		}
		rr, err := kc.GetRealmRole(ctx, realmName, name)
		if err != nil || rr == nil {
			return nil, fmt.Errorf("composite realm role %q not found in realm %q: %w", name, realmName, err)
		}
		resolved = append(resolved, *rr)
	}
	for clientID, names := range desired.Client {
		if clientID == "" || len(names) == 0 {
			continue
		}
		client, err := kc.GetClientByClientID(ctx, realmName, clientID)
		if err != nil || client == nil || client.ID == nil {
			return nil, fmt.Errorf("composite client %q not found in realm %q: %w", clientID, realmName, err)
		}
		for _, name := range names {
			if name == "" {
				continue
			}
			rr, err := kc.GetClientRole(ctx, realmName, *client.ID, name)
			if err != nil || rr == nil {
				return nil, fmt.Errorf("composite client role %q on client %q not found: %w", name, clientID, err)
			}
			resolved = append(resolved, *rr)
		}
	}
	return resolved, nil
}

// getKeycloakClientAndRealm resolves the Keycloak connection, the realm the role
// belongs to, and — for client roles — the owning client's UUID. A client role
// carries only clientRef; its realm is taken from the referenced client so the
// realm is never stated twice.
func (r *KeycloakRoleReconciler) getKeycloakClientAndRealm(ctx context.Context, role *keycloakv1beta1.KeycloakRole) (*keycloak.Client, string, string, error) {
	if role.Spec.ClientRef != nil {
		return r.getKeycloakClientAndRealmFromClient(ctx, role)
	}

	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, role.Namespace, role.Spec.RealmRef, role.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", "", err
	}
	return res.Client, res.RealmName, "", nil
}

// getKeycloakClientAndRealmFromClient resolves a client role's realm by following
// the referenced client's own realm reference.
func (r *KeycloakRoleReconciler) getKeycloakClientAndRealmFromClient(ctx context.Context, role *keycloakv1beta1.KeycloakRole) (*keycloak.Client, string, string, error) {
	clientKey := types.NamespacedName{
		Name:      role.Spec.ClientRef.Name,
		Namespace: role.Namespace,
	}

	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, clientKey, kcClient); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakClient %s: %w", clientKey, err)
	}

	if !kcClient.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakClient %s is not ready", clientKey)
	}

	if kcClient.Status.ClientUUID == "" {
		return nil, "", "", fmt.Errorf("KeycloakClient %s has no clientUUID", clientKey)
	}

	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, kcClient.Namespace, kcClient.Spec.RealmRef, kcClient.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", "", err
	}
	return res.Client, res.RealmName, kcClient.Status.ClientUUID, nil
}

func (r *KeycloakRoleReconciler) deleteRole(ctx context.Context, role *keycloakv1beta1.KeycloakRole) error {
	kc, realmName, _, err := r.getKeycloakClientAndRealm(ctx, role)
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

	role.Status.Conditions = setReadyCondition(role.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, role, ready)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRole{}).
		Complete(r)
}
