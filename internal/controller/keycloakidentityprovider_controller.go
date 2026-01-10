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

// KeycloakIdentityProviderReconciler reconciles a KeycloakIdentityProvider object
type KeycloakIdentityProviderReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/finalizers,verbs=update

// Reconcile handles KeycloakIdentityProvider reconciliation
func (r *KeycloakIdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakIdentityProvider"

	// Fetch the KeycloakIdentityProvider
	idp := &keycloakv1beta1.KeycloakIdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, idp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakIdentityProvider")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, idp.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !idp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(idp, FinalizerName) {
			if err := r.deleteIdentityProvider(ctx, idp); err != nil {
				log.Error(err, "failed to delete identity provider from Keycloak")
			}

			controllerutil.RemoveFinalizer(idp, FinalizerName)
			if err := r.Update(ctx, idp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(idp, FinalizerName) {
		controllerutil.AddFinalizer(idp, FinalizerName)
		if err := r.Update(ctx, idp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, idp)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, idp, false, "RealmNotReady", err.Error(), "")
	}

	// Parse identity provider definition to extract alias
	var idpDef struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", fmt.Sprintf("Failed to parse identity provider definition: %v", err), "")
	}

	// Ensure alias is set
	alias := idpDef.Alias
	if alias == "" {
		// Default to metadata.name
		alias = idp.Name
	}

	// Prepare definition with alias set
	definition := setFieldInDefinition(idp.Spec.Definition.Raw, "alias", alias)

	// Check if identity provider exists by alias
	existingIdp, err := kc.GetIdentityProvider(ctx, realmName, alias)

	if err != nil || existingIdp == nil {
		// Identity provider doesn't exist, create it
		log.Info("creating identity provider", "alias", alias, "realm", realmName)
		_, err = kc.CreateIdentityProvider(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, idp, false, "CreateFailed", fmt.Sprintf("Failed to create identity provider: %v", err), "")
		}
		log.Info("identity provider created successfully", "alias", alias)
	} else {
		// Identity provider exists, update it
		log.Info("updating identity provider", "alias", alias, "realm", realmName)
		if err := kc.UpdateIdentityProvider(ctx, realmName, alias, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, idp, false, "UpdateFailed", fmt.Sprintf("Failed to update identity provider: %v", err), alias)
		}
		log.Info("identity provider updated successfully", "alias", alias)
	}

	// Update status
	idp.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s", realmName, alias)
	return r.updateStatus(ctx, idp, true, "Ready", "Identity provider synchronized", alias)
}

func (r *KeycloakIdentityProviderReconciler) getKeycloakClientAndRealm(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if idp.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, idp.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if idp.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := idp.Namespace
	if idp.Spec.RealmRef.Namespace != nil {
		realmNamespace = *idp.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      idp.Spec.RealmRef.Name,
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

func (r *KeycloakIdentityProviderReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

func (r *KeycloakIdentityProviderReconciler) deleteIdentityProvider(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, idp)
	if err != nil {
		return err
	}

	// Get alias from definition
	var idpDef struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
		return fmt.Errorf("failed to parse identity provider definition: %w", err)
	}

	alias := idpDef.Alias
	if alias == "" {
		alias = idp.Name
	}

	return kc.DeleteIdentityProvider(ctx, realmName, alias)
}

func (r *KeycloakIdentityProviderReconciler) updateStatus(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider, ready bool, status, message, alias string) (ctrl.Result, error) {
	idp.Status.Ready = ready
	idp.Status.Status = status
	idp.Status.Message = message

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
	for i, c := range idp.Status.Conditions {
		if c.Type == "Ready" {
			idp.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		idp.Status.Conditions = append(idp.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, idp); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakIdentityProvider{}).
		Complete(r)
}
