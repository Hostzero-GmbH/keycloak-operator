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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakRoleMappingReconciler reconciles a KeycloakRoleMapping object
type KeycloakRoleMappingReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrolemappings/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers,verbs=get;list;watch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakgroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients,verbs=get;list;watch

// Reconcile handles KeycloakRoleMapping reconciliation
func (r *KeycloakRoleMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakRoleMapping"

	// Fetch the KeycloakRoleMapping
	mapping := &keycloakv1beta1.KeycloakRoleMapping{}
	if err := r.Get(ctx, req.NamespacedName, mapping); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRoleMapping")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, mapping.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Validate spec
	if mapping.Spec.Subject.UserRef == nil && mapping.Spec.Subject.GroupRef == nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, mapping, false, "InvalidSpec", "Either userRef or groupRef must be specified", "", "", "", "")
	}
	if mapping.Spec.Role == nil && mapping.Spec.RoleRef == nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, mapping, false, "InvalidSpec", "Either role or roleRef must be specified", "", "", "", "")
	}

	// Handle deletion
	if !mapping.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mapping, FinalizerName) {
			if err := r.removeRoleMapping(ctx, mapping); err != nil {
				log.Error(err, "failed to remove role mapping")
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

	// Resolve the subject (user or group)
	subjectType, subjectID, realmName, kc, err := r.resolveSubject(ctx, mapping)
	if err != nil {
		RecordError(controllerName, "subject_not_ready")
		return r.updateStatus(ctx, mapping, false, "SubjectNotReady", err.Error(), subjectType, "", "", "")
	}

	// Resolve the role
	roleName, roleType, clientUUID, err := r.resolveRole(ctx, mapping, kc, realmName)
	if err != nil {
		RecordError(controllerName, "role_not_found")
		return r.updateStatus(ctx, mapping, false, "RoleNotFound", err.Error(), subjectType, subjectID, roleName, roleType)
	}

	// Get the role object
	var role *keycloak.RoleRepresentation
	if roleType == "client" {
		role, err = kc.GetClientRole(ctx, realmName, clientUUID, roleName)
	} else {
		role, err = kc.GetRealmRole(ctx, realmName, roleName)
	}
	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, mapping, false, "RoleNotFound", fmt.Sprintf("Failed to get role: %v", err), subjectType, subjectID, roleName, roleType)
	}

	// Apply the role mapping
	roles := []keycloak.RoleRepresentation{*role}
	if subjectType == "user" {
		if roleType == "client" {
			err = kc.AddClientRolesToUser(ctx, realmName, clientUUID, subjectID, roles)
		} else {
			err = kc.AddRealmRolesToUser(ctx, realmName, subjectID, roles)
		}
	} else {
		if roleType == "client" {
			err = kc.AddClientRolesToGroup(ctx, realmName, clientUUID, subjectID, roles)
		} else {
			err = kc.AddRealmRolesToGroup(ctx, realmName, subjectID, roles)
		}
	}

	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, mapping, false, "MappingFailed", fmt.Sprintf("Failed to add role mapping: %v", err), subjectType, subjectID, roleName, roleType)
	}

	log.Info("role mapping applied", "subject", subjectType, "subjectID", subjectID, "role", roleName, "roleType", roleType)

	// Update status with resource path
	var resourcePath string
	if subjectType == "user" {
		resourcePath = fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings", realmName, subjectID)
	} else {
		resourcePath = fmt.Sprintf("/admin/realms/%s/groups/%s/role-mappings", realmName, subjectID)
	}
	mapping.Status.ResourcePath = resourcePath

	return r.updateStatus(ctx, mapping, true, "Ready", "Role mapping applied", subjectType, subjectID, roleName, roleType)
}

func (r *KeycloakRoleMappingReconciler) resolveSubject(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) (string, string, string, *keycloak.Client, error) {
	if mapping.Spec.Subject.UserRef != nil {
		user, err := r.getUser(ctx, mapping)
		if err != nil {
			return "user", "", "", nil, err
		}
		if !user.Status.Ready || user.Status.UserID == "" {
			return "user", "", "", nil, fmt.Errorf("user %s is not ready", user.Name)
		}

		kc, realmName, err := r.getKeycloakClientFromUser(ctx, user)
		if err != nil {
			return "user", "", "", nil, err
		}

		return "user", user.Status.UserID, realmName, kc, nil
	}

	if mapping.Spec.Subject.GroupRef != nil {
		group, err := r.getGroup(ctx, mapping)
		if err != nil {
			return "group", "", "", nil, err
		}
		if !group.Status.Ready || group.Status.GroupID == "" {
			return "group", "", "", nil, fmt.Errorf("group %s is not ready", group.Name)
		}

		kc, realmName, err := r.getKeycloakClientFromGroup(ctx, group)
		if err != nil {
			return "group", "", "", nil, err
		}

		return "group", group.Status.GroupID, realmName, kc, nil
	}

	return "", "", "", nil, fmt.Errorf("no subject specified")
}

func (r *KeycloakRoleMappingReconciler) resolveRole(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping, kc *keycloak.Client, realmName string) (string, string, string, error) {
	if mapping.Spec.Role != nil {
		roleName := mapping.Spec.Role.Name

		// Check if it's a client role
		if mapping.Spec.Role.ClientRef != nil || mapping.Spec.Role.ClientID != nil {
			var clientUUID string

			if mapping.Spec.Role.ClientRef != nil {
				// Resolve client reference
				client, err := r.getClient(ctx, mapping, mapping.Spec.Role.ClientRef)
				if err != nil {
					return roleName, "client", "", err
				}
				if !client.Status.Ready || client.Status.ClientUUID == "" {
					return roleName, "client", "", fmt.Errorf("client %s is not ready", client.Name)
				}
				clientUUID = client.Status.ClientUUID
			} else {
				// Use clientID directly - need to look up the UUID
				clients, err := kc.GetClients(ctx, realmName, map[string]string{
					"clientId": *mapping.Spec.Role.ClientID,
				})
				if err != nil || len(clients) == 0 {
					return roleName, "client", "", fmt.Errorf("client %s not found", *mapping.Spec.Role.ClientID)
				}
				clientUUID = *clients[0].ID
			}

			return roleName, "client", clientUUID, nil
		}

		return roleName, "realm", "", nil
	}

	// RoleRef - not implemented yet, would need a KeycloakRole CRD
	return "", "", "", fmt.Errorf("roleRef not yet supported")
}

func (r *KeycloakRoleMappingReconciler) getUser(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) (*keycloakv1beta1.KeycloakUser, error) {
	ref := mapping.Spec.Subject.UserRef
	namespace := mapping.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	user := &keycloakv1beta1.KeycloakUser{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, user); err != nil {
		return nil, fmt.Errorf("failed to get user %s/%s: %w", namespace, ref.Name, err)
	}
	return user, nil
}

func (r *KeycloakRoleMappingReconciler) getGroup(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) (*keycloakv1beta1.KeycloakGroup, error) {
	ref := mapping.Spec.Subject.GroupRef
	namespace := mapping.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	group := &keycloakv1beta1.KeycloakGroup{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, group); err != nil {
		return nil, fmt.Errorf("failed to get group %s/%s: %w", namespace, ref.Name, err)
	}
	return group, nil
}

func (r *KeycloakRoleMappingReconciler) getClient(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping, ref *keycloakv1beta1.ResourceRef) (*keycloakv1beta1.KeycloakClient, error) {
	namespace := mapping.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	client := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, client); err != nil {
		return nil, fmt.Errorf("failed to get client %s/%s: %w", namespace, ref.Name, err)
	}
	return client, nil
}

func (r *KeycloakRoleMappingReconciler) getKeycloakClientFromUser(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if user.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, user.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if user.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("user %s has no realmRef or clusterRealmRef", user.Name)
	}

	// Get the realm
	realmNamespace := user.Namespace
	if user.Spec.RealmRef.Namespace != nil {
		realmNamespace = *user.Spec.RealmRef.Namespace
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: user.Spec.RealmRef.Name, Namespace: realmNamespace}, realm); err != nil {
		return nil, "", err
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("realm %s is not ready", realm.Name)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get the instance
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}

	instanceName := types.NamespacedName{Name: realm.Spec.InstanceRef.Name, Namespace: instanceNamespace}
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", err
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("instance %s is not ready", instance.Name)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("failed to get Keycloak client")
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakRoleMappingReconciler) getKeycloakClientFromGroup(ctx context.Context, group *keycloakv1beta1.KeycloakGroup) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if group.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, group.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if group.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("group %s has no realmRef or clusterRealmRef", group.Name)
	}

	realmNamespace := group.Namespace
	if group.Spec.RealmRef.Namespace != nil {
		realmNamespace = *group.Spec.RealmRef.Namespace
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: group.Spec.RealmRef.Name, Namespace: realmNamespace}, realm); err != nil {
		return nil, "", err
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("realm %s is not ready", realm.Name)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get the instance
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}

	instanceName := types.NamespacedName{Name: realm.Spec.InstanceRef.Name, Namespace: instanceNamespace}
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", err
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("instance %s is not ready", instance.Name)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("failed to get Keycloak client")
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakRoleMappingReconciler) removeRoleMapping(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping) error {
	log := log.FromContext(ctx)

	// Resolve the subject
	subjectType, subjectID, realmName, kc, err := r.resolveSubject(ctx, mapping)
	if err != nil {
		log.Error(err, "failed to resolve subject for cleanup")
		return nil // Don't block deletion
	}

	// Resolve the role
	roleName, roleType, clientUUID, err := r.resolveRole(ctx, mapping, kc, realmName)
	if err != nil {
		log.Error(err, "failed to resolve role for cleanup")
		return nil
	}

	// Get the role object
	var role *keycloak.RoleRepresentation
	if roleType == "client" {
		role, err = kc.GetClientRole(ctx, realmName, clientUUID, roleName)
	} else {
		role, err = kc.GetRealmRole(ctx, realmName, roleName)
	}
	if err != nil {
		log.Error(err, "failed to get role for cleanup")
		return nil
	}

	roles := []keycloak.RoleRepresentation{*role}
	if subjectType == "user" {
		if roleType == "client" {
			err = kc.DeleteClientRolesFromUser(ctx, realmName, clientUUID, subjectID, roles)
		} else {
			err = kc.DeleteRealmRolesFromUser(ctx, realmName, subjectID, roles)
		}
	} else {
		if roleType == "client" {
			err = kc.DeleteClientRolesFromGroup(ctx, realmName, clientUUID, subjectID, roles)
		} else {
			err = kc.DeleteRealmRolesFromGroup(ctx, realmName, subjectID, roles)
		}
	}

	if err != nil {
		log.Error(err, "failed to remove role mapping")
	}

	return nil
}

func (r *KeycloakRoleMappingReconciler) updateStatus(ctx context.Context, mapping *keycloakv1beta1.KeycloakRoleMapping, ready bool, status, message, subjectType, subjectID, roleName, roleType string) (ctrl.Result, error) {
	mapping.Status.Ready = ready
	mapping.Status.Status = status
	mapping.Status.Message = message
	mapping.Status.SubjectType = subjectType
	mapping.Status.SubjectID = subjectID
	mapping.Status.RoleName = roleName
	mapping.Status.RoleType = roleType

	// Track observed generation to detect spec changes
	if ready {
		mapping.Status.ObservedGeneration = mapping.Generation
	}

	if err := r.Status().Update(ctx, mapping); err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
	}

	// Requeue after 5 minutes for periodic check
	return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
}

func (r *KeycloakRoleMappingReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRoleMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRoleMapping{}).
		Watches(
			&keycloakv1beta1.KeycloakUser{},
			handler.EnqueueRequestsFromMapFunc(r.findRoleMappingsForUser),
		).
		Watches(
			&keycloakv1beta1.KeycloakGroup{},
			handler.EnqueueRequestsFromMapFunc(r.findRoleMappingsForGroup),
		).
		Complete(r)
}

// findRoleMappingsForUser returns reconcile requests for all role mappings referencing the given user
func (r *KeycloakRoleMappingReconciler) findRoleMappingsForUser(ctx context.Context, obj client.Object) []reconcile.Request {
	user := obj.(*keycloakv1beta1.KeycloakUser)
	var mappings keycloakv1beta1.KeycloakRoleMappingList
	if err := r.List(ctx, &mappings, client.InNamespace(user.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, mapping := range mappings.Items {
		if mapping.Spec.Subject.UserRef != nil && mapping.Spec.Subject.UserRef.Name == user.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      mapping.Name,
					Namespace: mapping.Namespace,
				},
			})
		}
	}
	return requests
}

// findRoleMappingsForGroup returns reconcile requests for all role mappings referencing the given group
func (r *KeycloakRoleMappingReconciler) findRoleMappingsForGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	group := obj.(*keycloakv1beta1.KeycloakGroup)
	var mappings keycloakv1beta1.KeycloakRoleMappingList
	if err := r.List(ctx, &mappings, client.InNamespace(group.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, mapping := range mappings.Items {
		if mapping.Spec.Subject.GroupRef != nil && mapping.Spec.Subject.GroupRef.Name == group.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      mapping.Name,
					Namespace: mapping.Namespace,
				},
			})
		}
	}
	return requests
}
