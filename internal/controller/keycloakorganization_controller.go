package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

const (
	// MinKeycloakVersionForOrganizations is the minimum version that supports organizations
	MinKeycloakVersionForOrganizations = 26
)

// KeycloakOrganizationReconciler reconciles a KeycloakOrganization object
type KeycloakOrganizationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations/finalizers,verbs=update

// Reconcile handles KeycloakOrganization reconciliation
func (r *KeycloakOrganizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakOrganization"

	// Fetch the KeycloakOrganization
	org := &keycloakv1beta1.KeycloakOrganization{}
	if err := r.Get(ctx, req.NamespacedName, org); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakOrganization")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, org.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !org.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(org, FinalizerName) {
			if err := r.deleteOrganization(ctx, org); err != nil {
				log.Error(err, "failed to delete organization from Keycloak")
			}

			controllerutil.RemoveFinalizer(org, FinalizerName)
			if err := r.Update(ctx, org); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(org, FinalizerName) {
		controllerutil.AddFinalizer(org, FinalizerName)
		if err := r.Update(ctx, org); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client, realm info, and version
	kc, realmName, keycloakVersion, err := r.getKeycloakClientRealmAndVersion(ctx, org)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, org, false, "RealmNotReady", err.Error(), "")
	}

	// Check Keycloak version - organizations require >= 26
	if err := r.checkKeycloakVersionForOrganizations(keycloakVersion); err != nil {
		RecordError(controllerName, "version_unsupported")
		return r.updateStatus(ctx, org, false, "VersionUnsupported", err.Error(), "")
	}

	// Parse organization definition
	var orgDef keycloak.OrganizationRepresentation
	if err := json.Unmarshal(org.Spec.Definition.Raw, &orgDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, org, false, "InvalidDefinition", fmt.Sprintf("Failed to parse organization definition: %v", err), "")
	}

	// Ensure name is set
	if orgDef.Name == "" {
		orgDef.Name = org.Name
	}

	// Check if organization exists by name
	existingOrgs, err := kc.GetOrganizations(ctx, realmName)
	if err != nil {
		log.Error(err, "failed to list organizations", "realm", realmName)
		// Don't fail - might be first organization or organizations not enabled
	}
	var existingOrg *keycloak.OrganizationRepresentation
	if err == nil {
		for i := range existingOrgs {
			if existingOrgs[i].Name == orgDef.Name {
				existingOrg = &existingOrgs[i]
				break
			}
		}
	}

	var orgID string
	if existingOrg == nil {
		// Organization doesn't exist, create it
		log.Info("creating organization", "name", orgDef.Name, "realm", realmName)
		orgID, err = kc.CreateOrganization(ctx, realmName, orgDef)
		if err != nil {
			log.Error(err, "failed to create organization in Keycloak", "name", orgDef.Name, "realm", realmName)
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, org, false, "CreateFailed", fmt.Sprintf("Failed to create organization: %v", err), "")
		}
		log.Info("organization created successfully", "name", orgDef.Name, "id", orgID)
	} else {
		// Organization exists, update it
		orgID = existingOrg.ID
		orgDef.ID = orgID

		log.Info("updating organization", "name", orgDef.Name, "realm", realmName)
		if err := kc.UpdateOrganization(ctx, realmName, orgDef); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, org, false, "UpdateFailed", fmt.Sprintf("Failed to update organization: %v", err), orgID)
		}
		log.Info("organization updated successfully", "name", orgDef.Name)
	}

	// Update status
	org.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/organizations/%s", realmName, orgID)
	return r.updateStatus(ctx, org, true, "Ready", "Organization synchronized", orgID)
}

func (r *KeycloakOrganizationReconciler) checkKeycloakVersionForOrganizations(version string) error {
	if version == "" {
		return fmt.Errorf("unable to determine Keycloak version - organizations require Keycloak %d.0.0 or later", MinKeycloakVersionForOrganizations)
	}

	// Parse major version
	cleanVersion := version
	if idx := strings.Index(version, "-"); idx > 0 {
		cleanVersion = version[:idx]
	}

	parts := strings.Split(cleanVersion, ".")
	if len(parts) < 1 {
		return fmt.Errorf("invalid version format: %s", version)
	}

	majorVersion, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid major version in %s: %w", version, err)
	}

	if majorVersion < MinKeycloakVersionForOrganizations {
		return fmt.Errorf("organizations require Keycloak %d.0.0 or later (detected: %s)", MinKeycloakVersionForOrganizations, version)
	}

	return nil
}

func (r *KeycloakOrganizationReconciler) getKeycloakClientRealmAndVersion(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization) (*keycloak.Client, string, string, error) {
	// Check if using cluster realm ref
	if org.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, org.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if org.Spec.RealmRef == nil {
		return nil, "", "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	// Get the realm reference
	realmNamespace := org.Namespace
	if org.Spec.RealmRef.Namespace != nil {
		realmNamespace = *org.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      org.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	// Get the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	// Check if realm is ready
	if !realm.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse realm definition: %w", err)
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
		return nil, "", "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	// Check if instance is ready
	if !instance.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	// Get Keycloak version from instance status
	keycloakVersion := instance.Status.Version

	// Get the Keycloak client from manager
	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmDef.Realm, keycloakVersion, nil
}

func (r *KeycloakOrganizationReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, string, error) {
	// Get the ClusterKeycloakRealm
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	// Get realm name
	realmName := clusterRealm.Status.RealmName
	if realmName == "" {
		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(clusterRealm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", "", fmt.Errorf("failed to parse cluster realm definition: %w", err)
		}
		realmName = realmDef.Realm
	}

	// Get Keycloak version and client from cluster instance
	if clusterRealm.Spec.ClusterInstanceRef != nil {
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		keycloakVersion := clusterInstance.Status.Version

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, clusterInstance)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, keycloakVersion, nil
	}

	// Use namespaced instance ref
	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", "", fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceName := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	keycloakVersion := instance.Status.Version

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, keycloakVersion, nil
}

func (r *KeycloakOrganizationReconciler) deleteOrganization(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization) error {
	kc, realmName, _, err := r.getKeycloakClientRealmAndVersion(ctx, org)
	if err != nil {
		return err
	}

	if org.Status.OrganizationID == "" {
		return nil // No organization ID stored, nothing to delete
	}

	return kc.DeleteOrganization(ctx, realmName, org.Status.OrganizationID)
}

func (r *KeycloakOrganizationReconciler) updateStatus(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization, ready bool, status, message, orgID string) (ctrl.Result, error) {
	org.Status.Ready = ready
	org.Status.Status = status
	org.Status.Message = message
	if orgID != "" {
		org.Status.OrganizationID = orgID
	}

	// Track observed generation to detect spec changes
	if ready {
		org.Status.ObservedGeneration = org.Generation
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
	for i, c := range org.Status.Conditions {
		if c.Type == "Ready" {
			org.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		org.Status.Conditions = append(org.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, org); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakOrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakOrganization{}).
		Complete(r)
}
