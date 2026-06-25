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

const (
	// Keycloak stores per-realm user profile configuration as a ComponentModel
	// using this provider type/provider ID pair. Components created through the
	// dedicated /users/profile Admin API may be unnamed, so name-based adoption
	// alone is not reliable for this provider.
	userProfileProviderType          = "org.keycloak.userprofile.UserProfileProvider"
	declarativeUserProfileProviderID = "declarative-user-profile"
)

type componentIdentity struct {
	Name         string
	ProviderID   string
	ProviderType string
	ParentID     string
}

// KeycloakComponentReconciler reconciles a KeycloakComponent object
type KeycloakComponentReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakcomponents/finalizers,verbs=update

// Reconcile handles KeycloakComponent reconciliation
func (r *KeycloakComponentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakComponent"

	// Fetch the KeycloakComponent
	component := &keycloakv1beta1.KeycloakComponent{}
	if err := r.Get(ctx, req.NamespacedName, component); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakComponent")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, component.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !component.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(component, FinalizerName) {
			// Delete component from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(component) {
				log.Info("preserving component in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteComponent(ctx, component); err != nil {
				log.Error(err, "failed to delete component from Keycloak")
			}

			controllerutil.RemoveFinalizer(component, FinalizerName)
			if err := r.Update(ctx, component); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(component, FinalizerName) {
		controllerutil.AddFinalizer(component, FinalizerName)
		if err := r.Update(ctx, component); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, realmID, err := r.getKeycloakClientAndRealm(ctx, component)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, component, false, "RealmNotReady", err.Error(), "", "", "")
	}

	// Parse component definition to extract identity fields
	var componentDef struct {
		Name         string `json:"name"`
		ProviderID   string `json:"providerId"`
		ProviderType string `json:"providerType"`
		ParentID     string `json:"parentId"`
	}
	if err := json.Unmarshal(component.Spec.Definition.Raw, &componentDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, component, false, "InvalidDefinition", fmt.Sprintf("Failed to parse component definition: %v", err), "", "", "")
	}

	// Ensure name is set
	if componentDef.Name == "" {
		componentDef.Name = component.Name
	}

	// Prepare definition JSON with name set
	definition := setFieldInDefinition(component.Spec.Definition.Raw, "name", componentDef.Name)

	// Set parent ID to realm ID if not specified
	if componentDef.ParentID == "" {
		componentDef.ParentID = realmID
		definition = setFieldInDefinition(definition, "parentId", realmID)
	}

	// Resolve an existing Keycloak component before deciding whether to create
	// one. Most component types are identified well enough by name+providerType.
	// Declarative user-profile components need a fallback because Keycloak may
	// create them through the User Profile UI/API without a name.
	componentID, err := r.findExistingComponentID(ctx, kc, realmName, componentIdentity{
		Name:         componentDef.Name,
		ProviderID:   componentDef.ProviderID,
		ProviderType: componentDef.ProviderType,
		ParentID:     componentDef.ParentID,
	})
	if err != nil {
		RecordError(controllerName, "component_lookup_error")
		return r.updateStatus(ctx, component, false, "LookupFailed", err.Error(), "", componentDef.Name, componentDef.ProviderType)
	}

	if componentID == "" {
		// Create component
		log.Info("creating component", "name", componentDef.Name, "realm", realmName)
		componentID, err = kc.CreateComponent(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, component, false, "CreateFailed", fmt.Sprintf("Failed to create component: %v", err), "", "", "")
		}
		log.Info("component created successfully", "name", componentDef.Name, "id", componentID)
	} else {
		// Update component
		definition = mergeIDIntoDefinition(definition, &componentID)
		log.Info("updating component", "name", componentDef.Name, "realm", realmName)
		if err := kc.UpdateComponent(ctx, realmName, componentID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, component, false, "UpdateFailed", fmt.Sprintf("Failed to update component: %v", err), componentID, componentDef.Name, componentDef.ProviderType)
		}
		log.Info("component updated successfully", "name", componentDef.Name)
	}

	// Update status
	component.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/components/%s", realmName, componentID)
	return r.updateStatus(ctx, component, true, "Ready", "Component synchronized", componentID, componentDef.Name, componentDef.ProviderType)
}

// findExistingComponentID returns the Keycloak ID of the component represented
// by the CR, or an empty string when it does not exist yet.
//
// The normal component identity used by this controller is name+providerType.
// That keeps existing behavior for generic components such as keys and LDAP.
//
// A special fallback is needed for declarative user-profile components. When a
// user saves Realm settings -> User profile in the Keycloak Admin UI (or calls
// PUT /admin/realms/{realm}/users/profile), Keycloak persists the configuration
// as a ComponentModel with providerId=declarative-user-profile and
// providerType=org.keycloak.userprofile.UserProfileProvider, but the component
// can be unnamed. Matching by provider identity plus parent realm lets the
// operator adopt that existing component instead of creating a duplicate.
func (r *KeycloakComponentReconciler) findExistingComponentID(ctx context.Context, kc *keycloak.Client, realmName string, desired componentIdentity) (string, error) {
	// Fast path and backwards-compatible behavior: find components by the
	// configured name, then require providerType to match before adopting it.
	components, err := kc.GetComponents(ctx, realmName, map[string]string{"name": desired.Name})
	if err != nil {
		return "", err
	}
	componentID, err := findMatchingComponentID(components, desired)
	if err != nil || componentID != "" {
		return componentID, err
	}

	if desired.ProviderID != declarativeUserProfileProviderID || desired.ProviderType != userProfileProviderType {
		return "", nil
	}

	// User-profile fallback: query all user-profile components in the realm and
	// match the exact provider identity under this realm's parent ID. This is
	// intentionally narrow to avoid changing matching semantics for other
	// component types that may legitimately have repeated provider IDs.
	components, err = kc.GetComponents(ctx, realmName, map[string]string{"type": desired.ProviderType})
	if err != nil {
		return "", err
	}
	return findMatchingComponentID(components, desired)
}

func findMatchingComponentID(components []keycloak.ComponentRepresentation, desired componentIdentity) (string, error) {
	if componentID := findComponentByNameAndProviderType(components, desired); componentID != "" {
		return componentID, nil
	}

	if !desired.isDeclarativeUserProfile() {
		return "", nil
	}

	return findDeclarativeUserProfileComponent(components, desired)
}

func findComponentByNameAndProviderType(components []keycloak.ComponentRepresentation, desired componentIdentity) string {
	if desired.Name == "" || desired.ProviderType == "" {
		return ""
	}

	for _, c := range components {
		if c.ID == nil || c.Name == nil || c.ProviderType == nil {
			continue
		}
		if *c.Name == desired.Name && *c.ProviderType == desired.ProviderType {
			return *c.ID
		}
	}
	return ""
}

func findDeclarativeUserProfileComponent(components []keycloak.ComponentRepresentation, desired componentIdentity) (string, error) {
	var matches []string
	for _, c := range components {
		if c.ID == nil || c.ProviderID == nil || c.ProviderType == nil || c.ParentID == nil {
			continue
		}
		if *c.ProviderID == desired.ProviderID && *c.ProviderType == desired.ProviderType && *c.ParentID == desired.ParentID {
			matches = append(matches, *c.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple matching user profile components found for providerId=%q providerType=%q parentId=%q", desired.ProviderID, desired.ProviderType, desired.ParentID)
	}
}

func (c componentIdentity) isDeclarativeUserProfile() bool {
	return c.ProviderID == declarativeUserProfileProviderID && c.ProviderType == userProfileProviderType
}

func (r *KeycloakComponentReconciler) getKeycloakClientAndRealm(ctx context.Context, component *keycloakv1beta1.KeycloakComponent) (*keycloak.Client, string, string, error) {
	// Check if using cluster realm ref
	if component.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, component.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if component.Spec.RealmRef == nil {
		return nil, "", "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmName := types.NamespacedName{
		Name:      component.Spec.RealmRef.Name,
		Namespace: component.Namespace,
	}

	// Get the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	if !realm.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	kc, _, err := GetKeycloakClientFromRealmInstance(ctx, r.Client, r.ClientManager, realm)
	if err != nil {
		return nil, "", "", err
	}

	// Get the realm ID from Keycloak if not in definition
	realmID := realmDef.ID
	if realmID == "" {
		kcRealm, err := kc.GetRealm(ctx, realmDef.Realm)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get realm ID: %w", err)
		}
		if kcRealm.ID != nil {
			realmID = *kcRealm.ID
		} else {
			realmID = realmDef.Realm // Fall back to realm name
		}
	}

	return kc, realmDef.Realm, realmID, nil
}

func (r *KeycloakComponentReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, string, error) {
	// Get the ClusterKeycloakRealm
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	// Get realm name
	var realmDef struct {
		Realm string `json:"realm"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(clusterRealm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse cluster realm definition: %w", err)
	}

	realmName := clusterRealm.Status.RealmName
	if realmName == "" {
		realmName = realmDef.Realm
	}

	// Get Keycloak client from cluster instance
	if clusterRealm.Spec.ClusterInstanceRef != nil {
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, clusterInstance)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		// Get realm ID
		realmID := realmDef.ID
		if realmID == "" {
			kcRealm, err := kc.GetRealm(ctx, realmName)
			if err != nil {
				return nil, "", "", fmt.Errorf("failed to get realm ID: %w", err)
			}
			if kcRealm.ID != nil {
				realmID = *kcRealm.ID
			} else {
				realmID = realmName
			}
		}

		return kc, realmName, realmID, nil
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

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	// Get realm ID
	realmID := realmDef.ID
	if realmID == "" {
		kcRealm, err := kc.GetRealm(ctx, realmName)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to get realm ID: %w", err)
		}
		if kcRealm.ID != nil {
			realmID = *kcRealm.ID
		} else {
			realmID = realmName
		}
	}

	return kc, realmName, realmID, nil
}

func (r *KeycloakComponentReconciler) deleteComponent(ctx context.Context, component *keycloakv1beta1.KeycloakComponent) error {
	if component.Status.ComponentID == "" {
		return nil
	}

	kc, realmName, _, err := r.getKeycloakClientAndRealm(ctx, component)
	if err != nil {
		return err
	}

	return kc.DeleteComponent(ctx, realmName, component.Status.ComponentID)
}

func (r *KeycloakComponentReconciler) updateStatus(ctx context.Context, component *keycloakv1beta1.KeycloakComponent, ready bool, status, message, componentID, componentName, providerType string) (ctrl.Result, error) {
	component.Status.Ready = ready
	component.Status.Status = status
	component.Status.Message = message
	component.Status.ComponentID = componentID
	component.Status.ComponentName = componentName
	component.Status.ProviderType = providerType

	if ready {
		component.Status.ObservedGeneration = component.Generation
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
	for i, c := range component.Status.Conditions {
		if c.Type == "Ready" {
			component.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		component.Status.Conditions = append(component.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, component); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakComponent{}).
		Complete(r)
}
