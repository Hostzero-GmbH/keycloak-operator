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
			if err := r.deleteUser(ctx, user); err != nil {
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

	// Parse user definition to extract username
	var userDef struct {
		Username string `json:"username"`
	}
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
		// User exists, update it
		existingUser := existingUsers[0]
		userID = *existingUser.ID
		definition = mergeIDIntoDefinition(definition, existingUser.ID)

		log.Info("updating user", "username", username, "realm", realmName)
		if err := kc.UpdateUser(ctx, realmName, userID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, user, false, "UpdateFailed", fmt.Sprintf("Failed to update user: %v", err), userID, false, "")
		}
		log.Info("user updated successfully", "username", username)
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
	realmNamespace := user.Namespace
	if user.Spec.RealmRef.Namespace != nil {
		realmNamespace = *user.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: realmNamespace,
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

	// Get instance reference from realm
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	// Get the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	// Check if instance is ready
	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	// Get the Keycloak client from manager
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

		log.Info("updating service account user", "userID", userID, "realm", realmName)
		if err := kc.UpdateUser(ctx, realmName, userID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, user, false, "UpdateFailed", fmt.Sprintf("Failed to update service account user: %v", err), userID, true, clientUUID)
		}
		log.Info("service account user updated successfully", "userID", userID)
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
	clientNamespace := user.Namespace
	if user.Spec.ClientRef.Namespace != nil {
		clientNamespace = *user.Spec.ClientRef.Namespace
	}
	clientName := types.NamespacedName{
		Name:      user.Spec.ClientRef.Name,
		Namespace: clientNamespace,
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
		realmNamespace := kcClient.Namespace
		if kcClient.Spec.RealmRef.Namespace != nil {
			realmNamespace = *kcClient.Spec.RealmRef.Namespace
		}
		realmKey := types.NamespacedName{
			Name:      kcClient.Spec.RealmRef.Name,
			Namespace: realmNamespace,
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

		// Get instance
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
			return nil, "", "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
		}

		if !instance.Status.Ready {
			return nil, "", "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
		}

		cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get Keycloak config: %w", err)
		}

		kc = r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
		if kc == nil {
			return nil, "", "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
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
	user.Status.Ready = ready
	user.Status.Status = status
	user.Status.Message = message
	if userID != "" {
		user.Status.UserID = userID
	}
	user.Status.IsServiceAccount = isServiceAccount
	user.Status.ClientID = clientID

	// Track observed generation to detect spec changes
	if ready {
		user.Status.ObservedGeneration = user.Generation
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
	for i, c := range user.Status.Conditions {
		if c.Type == "Ready" {
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
