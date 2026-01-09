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

// KeycloakProtocolMapperReconciler reconciles a KeycloakProtocolMapper object
type KeycloakProtocolMapperReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers/finalizers,verbs=update

// Reconcile handles KeycloakProtocolMapper reconciliation
func (r *KeycloakProtocolMapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakProtocolMapper"

	// Fetch the KeycloakProtocolMapper
	mapper := &keycloakv1beta1.KeycloakProtocolMapper{}
	if err := r.Get(ctx, req.NamespacedName, mapper); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakProtocolMapper")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, mapper.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !mapper.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mapper, FinalizerName) {
			if err := r.deleteMapper(ctx, mapper); err != nil {
				log.Error(err, "failed to delete protocol mapper from Keycloak")
			}

			controllerutil.RemoveFinalizer(mapper, FinalizerName)
			if err := r.Update(ctx, mapper); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mapper, FinalizerName) {
		controllerutil.AddFinalizer(mapper, FinalizerName)
		if err := r.Update(ctx, mapper); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate that either clientRef or clientScopeRef is specified
	if mapper.Spec.ClientRef == nil && mapper.Spec.ClientScopeRef == nil {
		RecordError(controllerName, "invalid_spec")
		return r.updateStatus(ctx, mapper, false, "InvalidSpec", "Either clientRef or clientScopeRef must be specified", "", "", "", "")
	}

	// Get Keycloak client and realm info
	kc, realmName, parentType, parentID, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		RecordError(controllerName, "parent_not_ready")
		return r.updateStatus(ctx, mapper, false, "ParentNotReady", err.Error(), "", "", "", "")
	}

	// Parse mapper definition to extract name
	var mapperDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(mapper.Spec.Definition.Raw, &mapperDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, mapper, false, "InvalidDefinition", fmt.Sprintf("Failed to parse mapper definition: %v", err), "", "", "", "")
	}

	// Ensure name is set
	mapperName := mapperDef.Name
	if mapperName == "" {
		mapperName = mapper.Name
	}

	// Prepare definition with name set
	definition := setFieldInDefinition(mapper.Spec.Definition.Raw, "name", mapperName)

	// Find existing mapper by name
	var mapperID string
	if parentType == "client" {
		existingMappers, err := kc.GetClientProtocolMappers(ctx, realmName, parentID)
		if err == nil {
			for _, m := range existingMappers {
				if m.Name != nil && *m.Name == mapperName {
					mapperID = *m.ID
					break
				}
			}
		}
	} else {
		existingMappers, err := kc.GetClientScopeProtocolMappers(ctx, realmName, parentID)
		if err == nil {
			for _, m := range existingMappers {
				if m.Name != nil && *m.Name == mapperName {
					mapperID = *m.ID
					break
				}
			}
		}
	}

	if mapperID == "" {
		// Create mapper
		log.Info("creating protocol mapper", "name", mapperName, "realm", realmName, "parentType", parentType)
		var err error
		if parentType == "client" {
			mapperID, err = kc.CreateClientProtocolMapper(ctx, realmName, parentID, definition)
		} else {
			mapperID, err = kc.CreateClientScopeProtocolMapper(ctx, realmName, parentID, definition)
		}
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, mapper, false, "CreateFailed", fmt.Sprintf("Failed to create protocol mapper: %v", err), "", "", parentType, parentID)
		}
		log.Info("protocol mapper created successfully", "name", mapperName, "id", mapperID)
	} else {
		// Update mapper
		definition = mergeIDIntoDefinition(definition, &mapperID)
		log.Info("updating protocol mapper", "name", mapperName, "realm", realmName, "parentType", parentType)
		var err error
		if parentType == "client" {
			err = kc.UpdateClientProtocolMapper(ctx, realmName, parentID, mapperID, definition)
		} else {
			err = kc.UpdateClientScopeProtocolMapper(ctx, realmName, parentID, mapperID, definition)
		}
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, mapper, false, "UpdateFailed", fmt.Sprintf("Failed to update protocol mapper: %v", err), mapperID, mapperName, parentType, parentID)
		}
		log.Info("protocol mapper updated successfully", "name", mapperName)
	}

	// Update status
	if parentType == "client" {
		mapper.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models/%s", realmName, parentID, mapperID)
	} else {
		mapper.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/client-scopes/%s/protocol-mappers/models/%s", realmName, parentID, mapperID)
	}
	return r.updateStatus(ctx, mapper, true, "Ready", "Protocol mapper synchronized", mapperID, mapperName, parentType, parentID)
}

func (r *KeycloakProtocolMapperReconciler) getKeycloakClientAndParent(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) (*keycloak.Client, string, string, string, error) {
	if mapper.Spec.ClientRef != nil {
		return r.getFromClient(ctx, mapper)
	}
	return r.getFromClientScope(ctx, mapper)
}

func (r *KeycloakProtocolMapperReconciler) getFromClient(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) (*keycloak.Client, string, string, string, error) {
	clientNamespace := mapper.Namespace
	if mapper.Spec.ClientRef.Namespace != nil {
		clientNamespace = *mapper.Spec.ClientRef.Namespace
	}
	clientName := types.NamespacedName{
		Name:      mapper.Spec.ClientRef.Name,
		Namespace: clientNamespace,
	}

	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, clientName, kcClient); err != nil {
		return nil, "", "", "", fmt.Errorf("failed to get KeycloakClient %s: %w", clientName, err)
	}

	if !kcClient.Status.Ready {
		return nil, "", "", "", fmt.Errorf("KeycloakClient %s is not ready", clientName)
	}

	if kcClient.Status.ClientUUID == "" {
		return nil, "", "", "", fmt.Errorf("KeycloakClient %s has no clientUUID", clientName)
	}

	// Get realm from client
	kc, realmName, err := r.getKeycloakClientAndRealmFromClient(ctx, kcClient)
	if err != nil {
		return nil, "", "", "", err
	}

	return kc, realmName, "client", kcClient.Status.ClientUUID, nil
}

func (r *KeycloakProtocolMapperReconciler) getFromClientScope(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) (*keycloak.Client, string, string, string, error) {
	scopeNamespace := mapper.Namespace
	if mapper.Spec.ClientScopeRef.Namespace != nil {
		scopeNamespace = *mapper.Spec.ClientScopeRef.Namespace
	}
	scopeName := types.NamespacedName{
		Name:      mapper.Spec.ClientScopeRef.Name,
		Namespace: scopeNamespace,
	}

	scope := &keycloakv1beta1.KeycloakClientScope{}
	if err := r.Get(ctx, scopeName, scope); err != nil {
		return nil, "", "", "", fmt.Errorf("failed to get KeycloakClientScope %s: %w", scopeName, err)
	}

	if !scope.Status.Ready {
		return nil, "", "", "", fmt.Errorf("KeycloakClientScope %s is not ready", scopeName)
	}

	// Get scope ID from resource path
	scopeID := extractIDFromPath(scope.Status.ResourcePath)
	if scopeID == "" {
		return nil, "", "", "", fmt.Errorf("KeycloakClientScope %s has no ID in resource path", scopeName)
	}

	// Get realm from scope
	kc, realmName, err := r.getKeycloakClientAndRealmFromScope(ctx, scope)
	if err != nil {
		return nil, "", "", "", err
	}

	return kc, realmName, "clientScope", scopeID, nil
}

func (r *KeycloakProtocolMapperReconciler) getKeycloakClientAndRealmFromClient(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if kcClient.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, kcClient.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if kcClient.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("client has no realmRef or clusterRealmRef")
	}

	realmNamespace := kcClient.Namespace
	if kcClient.Spec.RealmRef.Namespace != nil {
		realmNamespace = *kcClient.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      kcClient.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get instance
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
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakProtocolMapperReconciler) getKeycloakClientAndRealmFromScope(ctx context.Context, scope *keycloakv1beta1.KeycloakClientScope) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if scope.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, scope.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if scope.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("scope has no realmRef or clusterRealmRef")
	}

	realmNamespace := scope.Namespace
	if scope.Spec.RealmRef.Namespace != nil {
		realmNamespace = *scope.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      scope.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get instance
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
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakProtocolMapperReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

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
			return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, nil
	}

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
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, nil
}

func (r *KeycloakProtocolMapperReconciler) deleteMapper(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) error {
	if mapper.Status.MapperID == "" || mapper.Status.ParentID == "" {
		return nil
	}

	kc, realmName, parentType, _, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		return err
	}

	if parentType == "client" {
		return kc.DeleteClientProtocolMapper(ctx, realmName, mapper.Status.ParentID, mapper.Status.MapperID)
	}
	return kc.DeleteClientScopeProtocolMapper(ctx, realmName, mapper.Status.ParentID, mapper.Status.MapperID)
}

func (r *KeycloakProtocolMapperReconciler) updateStatus(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper, ready bool, status, message, mapperID, mapperName, parentType, parentID string) (ctrl.Result, error) {
	mapper.Status.Ready = ready
	mapper.Status.Status = status
	mapper.Status.Message = message
	mapper.Status.MapperID = mapperID
	mapper.Status.MapperName = mapperName
	mapper.Status.ParentType = parentType
	mapper.Status.ParentID = parentID

	if ready {
		mapper.Status.ObservedGeneration = mapper.Generation
	}

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
	for i, c := range mapper.Status.Conditions {
		if c.Type == "Ready" {
			mapper.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		mapper.Status.Conditions = append(mapper.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, mapper); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// extractIDFromPath extracts the ID from a resource path
func extractIDFromPath(path string) string {
	// Path format: /admin/realms/{realm}/client-scopes/{id}
	if path == "" {
		return ""
	}
	// Simple split by /
	result := []string{}
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	if len(result) > 0 {
		return result[len(result)-1]
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakProtocolMapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakProtocolMapper{}).
		Complete(r)
}
