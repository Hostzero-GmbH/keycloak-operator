package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	stderrors "errors"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// KeycloakUserReconciler reconciles a KeycloakUser object
type KeycloakUserReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers/finalizers,verbs=update

// Reconcile handles KeycloakUser reconciliation
func (r *KeycloakUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakUser"

	// Fetch the KeycloakUser
	user := &keycloakv1beta1.KeycloakUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakUser")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, user.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !user.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(user, FinalizerName) {
			// Delete user from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(user) {
				log.Info("preserving user in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteUser(ctx, user); err != nil {
				log.Error(err, "failed to delete user from Keycloak")
			}

			controllerutil.RemoveFinalizer(user, FinalizerName)
			if err := r.Update(ctx, user); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, FinalizerName) {
		controllerutil.AddFinalizer(user, FinalizerName)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if this is a service account user (belongs to a client)
	if user.IsServiceAccountUser() {
		return r.reconcileServiceAccountUser(ctx, user)
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, user)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, user, false, "RealmNotReady", err.Error(), "", false, "")
	}

	// Parse user definition to extract all fields
	var userDef keycloakv1beta1.UserDefinition
	if err := json.Unmarshal(user.Spec.Definition.Raw, &userDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, user, false, "InvalidDefinition", fmt.Sprintf("Failed to parse user definition: %v", err), "", false, "")
	}

	// Ensure username is set
	username := userDef.Username
	if username == "" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, user, false, "InvalidDefinition", "Username is required in definition", "", false, "")
	}

	// Prepare definition
	definition := user.Spec.Definition.Raw

	// Check if user exists by username
	existingUsers, err := kc.GetUsers(ctx, realmName, map[string]string{
		"username": username,
		"exact":    "true",
	})

	var userID string
	if err != nil || len(existingUsers) == 0 {
		// User doesn't exist, create it
		log.Info("creating user", "username", username, "realm", realmName)
		userID, err = kc.CreateUser(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, user, false, "CreateFailed", fmt.Sprintf("Failed to create user: %v", err), "", false, "")
		}
		log.Info("user created successfully", "username", username, "id", userID)
	} else {
		// User exists — check if update is needed
		existingUser := existingUsers[0]
		userID = *existingUser.ID
		definition = mergeIDIntoDefinition(definition, existingUser.ID)

		// Fetch current state for drift detection
		// Exclude role/group fields — they are reconciled separately via dedicated endpoints
		currentRaw, fetchErr := kc.GetUserRaw(ctx, realmName, userID)

		needsUpdate := true
		if fetchErr != nil {
			log.Error(fetchErr, "failed to fetch current user state, falling through to update")
		} else if currentRaw != nil {
			needsUpdate = !definitionsMatchStrict(definition, currentRaw, "id", "createdTimestamp", "access", "federatedIdentities", "origins", "credentials", "realmRoles", "clientRoles", "groups")
		}

		if needsUpdate {
			log.Info("updating user", "username", username, "realm", realmName)
			if err := kc.UpdateUser(ctx, realmName, userID, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, user, false, "UpdateFailed", fmt.Sprintf("Failed to update user: %v", err), userID, false, "")
			}
			log.Info("user updated successfully", "username", username)
		} else {
			log.V(1).Info("user already in sync, skipping update", "username", username)
		}
	}

	// Reconcile realm roles via dedicated role-mapping endpoints
	// nil = field omitted, don't manage; non-nil even if empty = reconcile to that set
	var roleGroupErrors []error
	if userDef.RealmRoles != nil {
		if err := r.reconcileUserRealmRoles(ctx, kc, realmName, userID, userDef.RealmRoles); err != nil {
			log.Error(err, "failed to reconcile realm roles", "username", username)
			roleGroupErrors = append(roleGroupErrors, fmt.Errorf("realm roles: %w", err))
		}
	}

	// Reconcile client roles via dedicated role-mapping endpoints
	if userDef.ClientRoles != nil {
		if err := r.reconcileUserClientRoles(ctx, kc, realmName, userID, userDef.ClientRoles); err != nil {
			log.Error(err, "failed to reconcile client roles", "username", username)
			roleGroupErrors = append(roleGroupErrors, fmt.Errorf("client roles: %w", err))
		}
	}

	// Reconcile group memberships via dedicated group membership endpoints
	if userDef.Groups != nil {
		if err := r.reconcileUserGroups(ctx, kc, realmName, userID, userDef.Groups); err != nil {
			log.Error(err, "failed to reconcile groups", "username", username)
			roleGroupErrors = append(roleGroupErrors, fmt.Errorf("groups: %w", err))
		}
	}

	if len(roleGroupErrors) > 0 {
		RecordError(controllerName, "role_group_reconcile_error")
		return r.updateStatus(ctx, user, false, "RoleGroupReconcileError",
			fmt.Sprintf("Failed to reconcile roles/groups: %v", stderrors.Join(roleGroupErrors...)), userID, false, "")
	}

	// Handle initial password if specified
	if user.Spec.InitialPassword != nil && user.Status.UserID == "" {
		// Only set password on first creation
		if err := kc.SetPassword(ctx, realmName, userID, user.Spec.InitialPassword.Value, user.Spec.InitialPassword.Temporary); err != nil {
			log.Error(err, "failed to set initial password")
			// Don't fail the reconciliation for password issues
		}
	}

	// Update status
	user.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/users/%s", realmName, userID)
	return r.updateStatus(ctx, user, true, "Ready", "User synchronized", userID, false, "")
}

func (r *KeycloakUserReconciler) getKeycloakClientAndRealm(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if user.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, user.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if user.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	// Get the realm reference
	realmName := types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: user.Namespace,
	}

	// Get the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	// Check if realm is ready
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

	kc, _, err := GetKeycloakClientFromRealmInstance(ctx, r.Client, r.ClientManager, realm)
	if err != nil {
		return nil, "", err
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakUserReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

func (r *KeycloakUserReconciler) deleteUser(ctx context.Context, user *keycloakv1beta1.KeycloakUser) error {
	// Service account users are managed by the client, don't delete them
	if user.Status.IsServiceAccount {
		log.FromContext(ctx).Info("skipping deletion of service account user - managed by client", "userID", user.Status.UserID)
		return nil
	}

	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, user)
	if err != nil {
		return err
	}

	if user.Status.UserID == "" {
		return nil // No user ID stored, nothing to delete
	}

	return kc.DeleteUser(ctx, realmName, user.Status.UserID)
}

func (r *KeycloakUserReconciler) reconcileUserRealmRoles(ctx context.Context, kc *keycloak.Client, realmName, userID string, roles []string) error {
	log := log.FromContext(ctx)
	log.V(1).Info("reconciling realm roles", "userID", userID, "count", len(roles))

	allRealmRoles, err := kc.GetRealmRoles(ctx, realmName)
	if err != nil {
		return fmt.Errorf("failed to list realm roles: %w", err)
	}
	roleByName := make(map[string]keycloak.RoleRepresentation)
	for _, rr := range allRealmRoles {
		if rr.Name != nil {
			roleByName[*rr.Name] = rr
		}
	}

	existing, err := kc.GetUserRealmRoleMappings(ctx, realmName, userID)
	if err != nil {
		return fmt.Errorf("failed to get existing realm role mappings: %w", err)
	}
	existingNames := make(map[string]bool)
	for _, r := range existing {
		if r.Name != nil {
			existingNames[*r.Name] = true
		}
	}

	want := make(map[string]bool)
	for _, role := range roles {
		want[role] = true
	}

	var toAdd, toRemove []keycloak.RoleRepresentation
	for _, name := range roles {
		if !existingNames[name] {
			if rr, ok := roleByName[name]; ok {
				toAdd = append(toAdd, rr)
			} else {
				log.Info("realm role not found, skipping", "role", name)
			}
		}
	}
	for _, r := range existing {
		if r.Name != nil && !want[*r.Name] {
			toRemove = append(toRemove, r)
		}
	}

	if len(toAdd) > 0 {
		if err := kc.AddRealmRolesToUser(ctx, realmName, userID, toAdd); err != nil {
			return fmt.Errorf("failed to add realm roles: %w", err)
		}
		log.V(1).Info("added realm roles", "count", len(toAdd))
	}
	if len(toRemove) > 0 {
		if err := kc.DeleteRealmRolesFromUser(ctx, realmName, userID, toRemove); err != nil {
			return fmt.Errorf("failed to remove realm roles: %w", err)
		}
		log.V(1).Info("removed realm roles", "count", len(toRemove))
	}

	return nil
}

func (r *KeycloakUserReconciler) reconcileUserClientRoles(ctx context.Context, kc *keycloak.Client, realmName, userID string, clientRoles map[string][]string) error {
	log := log.FromContext(ctx)
	log.V(1).Info("reconciling client roles", "userID", userID, "clients", len(clientRoles))

	// Resolve all wanted clients and collect their UUIDs
	type wantedClient struct {
		uuid  string
		roles []string
	}
	wanted := make(map[string]*wantedClient)
	wantedUUIDs := make(map[string]bool)
	var resolveErrs []error

	for clientID, roles := range clientRoles {
		clientRep, err := kc.GetClientByClientID(ctx, realmName, clientID)
		if err != nil {
			log.Error(err, "failed to resolve client, skipping roles", "clientID", clientID)
			resolveErrs = append(resolveErrs, fmt.Errorf("resolve client %s: %w", clientID, err))
			continue
		}
		if clientRep.ID == nil {
			log.Info("client has nil ID, skipping roles", "clientID", clientID)
			resolveErrs = append(resolveErrs, fmt.Errorf("client %s has nil ID", clientID))
			continue
		}
		wanted[clientID] = &wantedClient{uuid: *clientRep.ID, roles: roles}
		wantedUUIDs[*clientRep.ID] = true
	}

	// Reconcile roles per wanted client (handles both add and remove within each client)
	var reconcileErrs []error
	for _, wc := range wanted {
		allClientRoles, err := kc.GetClientRoles(ctx, realmName, wc.uuid)
		if err != nil {
			log.Error(err, "failed to list client roles, skipping", "client", wc.uuid)
			reconcileErrs = append(reconcileErrs, fmt.Errorf("list roles for client %s: %w", wc.uuid, err))
			continue
		}
		roleByName := make(map[string]keycloak.RoleRepresentation)
		for _, cr := range allClientRoles {
			if cr.Name != nil {
				roleByName[*cr.Name] = cr
			}
		}

		existing, err := kc.GetUserClientRoleMappings(ctx, realmName, userID, wc.uuid)
		if err != nil {
			log.Error(err, "failed to get existing client role mappings, skipping", "client", wc.uuid)
			reconcileErrs = append(reconcileErrs, fmt.Errorf("get role mappings for client %s: %w", wc.uuid, err))
			continue
		}

		existingNames := make(map[string]bool)
		for _, r := range existing {
			if r.Name != nil {
				existingNames[*r.Name] = true
			}
		}

		want := make(map[string]bool)
		for _, role := range wc.roles {
			want[role] = true
		}

		var toAdd, toRemove []keycloak.RoleRepresentation
		for _, name := range wc.roles {
			if !existingNames[name] {
				if cr, ok := roleByName[name]; ok {
					toAdd = append(toAdd, cr)
				} else {
					log.Info("client role not found in realm, skipping", "role", name)
				}
			}
		}
		for _, r := range existing {
			if r.Name != nil && !want[*r.Name] {
				toRemove = append(toRemove, r)
			}
		}

		if len(toAdd) > 0 {
			if err := kc.AddClientRolesToUser(ctx, realmName, wc.uuid, userID, toAdd); err != nil {
				log.Error(err, "failed to add client roles", "client", wc.uuid)
				reconcileErrs = append(reconcileErrs, fmt.Errorf("add roles to client %s: %w", wc.uuid, err))
				continue
			}
			log.V(1).Info("added client roles", "count", len(toAdd))
		}
		if len(toRemove) > 0 {
			if err := kc.DeleteClientRolesFromUser(ctx, realmName, wc.uuid, userID, toRemove); err != nil {
				log.Error(err, "failed to remove client roles", "client", wc.uuid)
				reconcileErrs = append(reconcileErrs, fmt.Errorf("remove roles from client %s: %w", wc.uuid, err))
				continue
			}
			log.V(1).Info("removed client roles", "count", len(toRemove))
		}
	}

	// Clean up stale client roles: users may have role mappings on clients
	// that are no longer in the wanted set (client key removed from spec).
	// Use the composite /role-mappings endpoint to get ALL client role mappings
	// in a single API call, avoiding the N+1 problem of iterating every realm client.
	composite, compErr := kc.GetUserRoleMappingsComposite(ctx, realmName, userID)
	if compErr != nil {
		reconcileErrs = append(reconcileErrs, fmt.Errorf("stale cleanup composite: %w", compErr))
	} else {
		for clientUUID, entry := range composite.ClientMappings {
			if wantedUUIDs[clientUUID] {
				continue
			}
			if len(entry.Mappings) == 0 {
				continue
			}
			if err := kc.DeleteClientRolesFromUser(ctx, realmName, clientUUID, userID, entry.Mappings); err != nil {
				log.Error(err, "failed to clean up stale client roles", "clientUUID", clientUUID)
				reconcileErrs = append(reconcileErrs, fmt.Errorf("stale cleanup for client %s: %w", clientUUID, err))
				continue
			}
			log.V(1).Info("cleaned up stale client roles", "count", len(entry.Mappings), "client", entry.Client)
		}
	}

	return stderrors.Join(append(resolveErrs, reconcileErrs...)...)
}

func (r *KeycloakUserReconciler) reconcileUserGroups(ctx context.Context, kc *keycloak.Client, realmName, userID string, groups []string) error {
	log := log.FromContext(ctx)
	log.V(1).Info("reconciling groups", "userID", userID, "count", len(groups))

	existingGroups, err := kc.GetUserGroups(ctx, realmName, userID)
	if err != nil {
		return fmt.Errorf("failed to get existing user groups: %w", err)
	}

	existingByName := make(map[string]string)
	for _, g := range existingGroups {
		if g.Name != nil && g.ID != nil {
			existingByName[*g.Name] = *g.ID
		}
	}

	allGroups, err := kc.GetGroups(ctx, realmName, nil)
	if err != nil {
		return fmt.Errorf("failed to list realm groups: %w", err)
	}

	groupByName := make(map[string]string)
	for _, g := range allGroups {
		if g.Name != nil && g.ID != nil {
			groupByName[*g.Name] = *g.ID
		}
	}

	want := make(map[string]bool)
	for _, name := range groups {
		want[name] = true
	}

	for _, name := range groups {
		if _, joined := existingByName[name]; !joined {
			groupID, found := groupByName[name]
			if !found {
				log.Info("group not found in realm, skipping", "group", name)
				continue
			}
			if err := kc.AddUserToGroup(ctx, realmName, userID, groupID); err != nil {
				log.Error(err, "failed to add user to group", "group", name)
				continue
			}
			log.V(1).Info("added to group", "group", name)
		}
	}

	for name, groupID := range existingByName {
		if !want[name] {
			if err := kc.RemoveUserFromGroup(ctx, realmName, userID, groupID); err != nil {
				log.Error(err, "failed to remove user from group", "group", name)
				continue
			}
			log.V(1).Info("removed from group", "group", name)
		}
	}

	return nil
}

func (r *KeycloakUserReconciler) reconcileServiceAccountUser(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	controllerName := "KeycloakUser"

	// Get the Keycloak client info from the referenced client
	kc, realmName, clientUUID, err := r.getKeycloakClientAndRealmFromClient(ctx, user)
	if err != nil {
		RecordError(controllerName, "client_not_ready")
		return r.updateStatus(ctx, user, false, "ClientNotReady", err.Error(), "", false, "")
	}

	// Get the service account user for this client
	serviceAccountUser, err := kc.GetClientServiceAccount(ctx, realmName, clientUUID)
	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, user, false, "ServiceAccountNotFound", fmt.Sprintf("Failed to get service account user: %v", err), "", true, clientUUID)
	}

	if serviceAccountUser == nil || serviceAccountUser.ID == nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, user, false, "ServiceAccountNotFound", "Service account user not found - ensure client has serviceAccountsEnabled: true", "", true, clientUUID)
	}

	userID := *serviceAccountUser.ID
	log.Info("found service account user", "userID", userID, "username", *serviceAccountUser.Username, "client", clientUUID)

	// If a definition is provided, update the service account user with it
	if user.Spec.Definition != nil && len(user.Spec.Definition.Raw) > 0 {
		// Merge ID and username into the definition to preserve service account identity
		definition := user.Spec.Definition.Raw
		definition = mergeIDIntoDefinition(definition, serviceAccountUser.ID)
		definition = setFieldInDefinition(definition, "username", *serviceAccountUser.Username)

		// Parse UserDefinition for role/group fields
		var userDef keycloakv1beta1.UserDefinition
		parseErr := json.Unmarshal(user.Spec.Definition.Raw, &userDef)
		if parseErr != nil {
			log.Error(parseErr, "failed to parse user definition for role/group reconciliation")
		}

		// Check for drift before updating
		// Exclude role/group fields — they are reconciled separately via dedicated endpoints
		currentRaw, fetchErr := kc.GetUserRaw(ctx, realmName, userID)
		needsUpdate := true
		if fetchErr != nil {
			log.Error(fetchErr, "failed to fetch current service account user state, falling through to update")
		} else if currentRaw != nil {
			needsUpdate = !definitionsMatchStrict(definition, currentRaw, "id", "createdTimestamp", "access", "federatedIdentities", "origins", "credentials", "realmRoles", "clientRoles", "groups")
		}

		if needsUpdate {
			log.Info("updating service account user", "userID", userID, "realm", realmName)
			if err := kc.UpdateUser(ctx, realmName, userID, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, user, false, "UpdateFailed", fmt.Sprintf("Failed to update service account user: %v", err), userID, true, clientUUID)
			}
			log.Info("service account user updated successfully", "userID", userID)
		} else {
			log.V(1).Info("service account user already in sync, skipping update", "userID", userID)
		}

		// Reconcile realm roles via dedicated endpoints
		var saRoleGroupErrors []error
		if parseErr == nil {
			if userDef.RealmRoles != nil {
				if err := r.reconcileUserRealmRoles(ctx, kc, realmName, userID, userDef.RealmRoles); err != nil {
					log.Error(err, "failed to reconcile realm roles for service account", "userID", userID)
					saRoleGroupErrors = append(saRoleGroupErrors, fmt.Errorf("realm roles: %w", err))
				}
			}
			if userDef.ClientRoles != nil {
				if err := r.reconcileUserClientRoles(ctx, kc, realmName, userID, userDef.ClientRoles); err != nil {
					log.Error(err, "failed to reconcile client roles for service account", "userID", userID)
					saRoleGroupErrors = append(saRoleGroupErrors, fmt.Errorf("client roles: %w", err))
				}
			}
			if userDef.Groups != nil {
				if err := r.reconcileUserGroups(ctx, kc, realmName, userID, userDef.Groups); err != nil {
					log.Error(err, "failed to reconcile groups for service account", "userID", userID)
					saRoleGroupErrors = append(saRoleGroupErrors, fmt.Errorf("groups: %w", err))
				}
			}
		}
		if len(saRoleGroupErrors) > 0 {
			RecordError(controllerName, "role_group_reconcile_error")
			return r.updateStatus(ctx, user, false, "RoleGroupReconcileError",
				fmt.Sprintf("Failed to reconcile roles/groups: %v", stderrors.Join(saRoleGroupErrors...)), userID, true, clientUUID)
		}
	}

	// Update status
	user.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/users/%s", realmName, userID)
	return r.updateStatus(ctx, user, true, "Ready", "Service account user synchronized", userID, true, clientUUID)
}

func (r *KeycloakUserReconciler) getKeycloakClientAndRealmFromClient(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (*keycloak.Client, string, string, error) {
	if user.Spec.ClientRef == nil {
		return nil, "", "", fmt.Errorf("clientRef is required for service account users")
	}

	// Get the KeycloakClient
	clientName := types.NamespacedName{
		Name:      user.Spec.ClientRef.Name,
		Namespace: user.Namespace,
	}

	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, clientName, kcClient); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakClient %s: %w", clientName, err)
	}

	if !kcClient.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakClient %s is not ready", clientName)
	}

	if kcClient.Status.ClientUUID == "" {
		return nil, "", "", fmt.Errorf("KeycloakClient %s has no clientUUID", clientName)
	}

	// Get realm from client
	var kc *keycloak.Client
	var realmName string
	var err error

	if kcClient.Spec.ClusterRealmRef != nil {
		kc, realmName, err = r.getKeycloakClientFromClusterRealm(ctx, kcClient.Spec.ClusterRealmRef.Name)
	} else if kcClient.Spec.RealmRef != nil {
		// Get the realm
		realmKey := types.NamespacedName{
			Name:      kcClient.Spec.RealmRef.Name,
			Namespace: kcClient.Namespace,
		}

		realm := &keycloakv1beta1.KeycloakRealm{}
		if err := r.Get(ctx, realmKey, realm); err != nil {
			return nil, "", "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmKey, err)
		}

		if !realm.Status.Ready {
			return nil, "", "", fmt.Errorf("KeycloakRealm %s is not ready", realmKey)
		}

		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", "", fmt.Errorf("failed to parse realm definition: %w", err)
		}
		realmName = realmDef.Realm

		kc, _, err = GetKeycloakClientFromRealmInstance(ctx, r.Client, r.ClientManager, realm)
		if err != nil {
			return nil, "", "", err
		}
	} else {
		return nil, "", "", fmt.Errorf("client %s has no realmRef or clusterRealmRef", clientName)
	}

	if err != nil {
		return nil, "", "", err
	}

	return kc, realmName, kcClient.Status.ClientUUID, nil
}

func (r *KeycloakUserReconciler) updateStatus(ctx context.Context, user *keycloakv1beta1.KeycloakUser, ready bool, status, message, userID string, isServiceAccount bool, clientID string) (ctrl.Result, error) {
	// Determine desired condition status
	desiredConditionStatus := metav1.ConditionFalse
	if ready {
		desiredConditionStatus = metav1.ConditionTrue
	}

	// Check if status actually changed
	statusChanged := user.Status.Ready != ready ||
		user.Status.Status != status ||
		user.Status.Message != message ||
		user.Status.IsServiceAccount != isServiceAccount ||
		user.Status.ClientID != clientID ||
		(userID != "" && user.Status.UserID != userID)

	conditionChanged := true
	for _, c := range user.Status.Conditions {
		if c.Type == "Ready" && c.Status == desiredConditionStatus && c.Reason == status && c.Message == message {
			conditionChanged = false
			break
		}
	}

	generationChanged := ready && user.Status.ObservedGeneration != user.Generation

	if !statusChanged && !conditionChanged && !generationChanged {
		if ready {
			return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
		}
		return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
	}

	user.Status.Ready = ready
	user.Status.Status = status
	user.Status.Message = message
	if userID != "" {
		user.Status.UserID = userID
	}
	user.Status.IsServiceAccount = isServiceAccount
	user.Status.ClientID = clientID

	if ready {
		user.Status.ObservedGeneration = user.Generation
	}

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             desiredConditionStatus,
		Reason:             status,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	found := false
	for i, c := range user.Status.Conditions {
		if c.Type == "Ready" {
			if c.Status == desiredConditionStatus {
				condition.LastTransitionTime = c.LastTransitionTime
			}
			user.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		user.Status.Conditions = append(user.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakUser{}).
		Watches(
			&keycloakv1beta1.KeycloakRealm{},
			handler.EnqueueRequestsFromMapFunc(r.findUsersForRealm),
		).
		Watches(
			&keycloakv1beta1.ClusterKeycloakRealm{},
			handler.EnqueueRequestsFromMapFunc(r.findUsersForClusterRealm),
		).
		Watches(
			&keycloakv1beta1.KeycloakClient{},
			handler.EnqueueRequestsFromMapFunc(r.findUsersForClient),
		).
		Complete(r)
}

// findUsersForRealm returns reconcile requests for all users referencing the given realm
func (r *KeycloakUserReconciler) findUsersForRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	realm := obj.(*keycloakv1beta1.KeycloakRealm)
	var users keycloakv1beta1.KeycloakUserList
	if err := r.List(ctx, &users, client.InNamespace(realm.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, user := range users.Items {
		if user.Spec.RealmRef != nil && user.Spec.RealmRef.Name == realm.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      user.Name,
					Namespace: user.Namespace,
				},
			})
		}
	}
	return requests
}

// findUsersForClusterRealm returns reconcile requests for all users referencing the given cluster realm
func (r *KeycloakUserReconciler) findUsersForClusterRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	realm := obj.(*keycloakv1beta1.ClusterKeycloakRealm)
	var users keycloakv1beta1.KeycloakUserList
	if err := r.List(ctx, &users); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, user := range users.Items {
		if user.Spec.ClusterRealmRef != nil && user.Spec.ClusterRealmRef.Name == realm.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      user.Name,
					Namespace: user.Namespace,
				},
			})
		}
	}
	return requests
}

// findUsersForClient returns reconcile requests for all users referencing the given client (service accounts)
func (r *KeycloakUserReconciler) findUsersForClient(ctx context.Context, obj client.Object) []reconcile.Request {
	kcClient := obj.(*keycloakv1beta1.KeycloakClient)
	var users keycloakv1beta1.KeycloakUserList
	if err := r.List(ctx, &users, client.InNamespace(kcClient.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, user := range users.Items {
		if user.Spec.ClientRef != nil && user.Spec.ClientRef.Name == kcClient.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      user.Name,
					Namespace: user.Namespace,
				},
			})
		}
	}
	return requests
}
