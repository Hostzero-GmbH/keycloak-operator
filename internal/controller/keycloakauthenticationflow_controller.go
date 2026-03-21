// This controller differs from other resource controllers in this operator.
// Most controllers pass a raw JSON definition to a single Keycloak API endpoint.
// Authentication flows require a sequence of procedural API calls because
// Keycloak provides no declarative PUT endpoint for complete flows. The
// controller translates the typed spec into calls to create the flow, add
// executions and sub-flows, set requirements, reorder by priority, and apply
// authenticator configs. See the types file for more context on this design.

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

// KeycloakAuthenticationFlowReconciler reconciles a KeycloakAuthenticationFlow object
type KeycloakAuthenticationFlowReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakauthenticationflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakauthenticationflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakauthenticationflows/finalizers,verbs=update

// Reconcile handles KeycloakAuthenticationFlow reconciliation
func (r *KeycloakAuthenticationFlowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakAuthenticationFlow"

	flow := &keycloakv1beta1.KeycloakAuthenticationFlow{}
	if err := r.Get(ctx, req.NamespacedName, flow); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakAuthenticationFlow")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	defer func() {
		RecordReconcile(controllerName, flow.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !flow.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(flow, FinalizerName) {
			if ShouldPreserveResource(flow) {
				log.Info("preserving flow in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteFlow(ctx, flow); err != nil {
				log.Error(err, "failed to delete authentication flow from Keycloak")
			}

			controllerutil.RemoveFinalizer(flow, FinalizerName)
			if err := r.Update(ctx, flow); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(flow, FinalizerName) {
		controllerutil.AddFinalizer(flow, FinalizerName)
		if err := r.Update(ctx, flow); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, flow)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, flow, false, "RealmNotReady", err.Error(), "")
	}

	// Find existing flow by alias
	existingFlowID, err := r.findFlowByAlias(ctx, kc, realmName, flow.Spec.Alias)
	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, flow, false, "APIError", fmt.Sprintf("Failed to list flows: %v", err), "")
	}

	if existingFlowID != "" {
		// Phase 1: on spec change, delete and recreate
		if flow.Status.ObservedGeneration != 0 && flow.Status.ObservedGeneration < flow.Generation {
			log.Info("spec changed, recreating flow", "alias", flow.Spec.Alias)
			if err := kc.DeleteAuthenticationFlow(ctx, realmName, existingFlowID); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, flow, false, "DeleteFailed", fmt.Sprintf("Failed to delete flow for recreation: %v", err), existingFlowID)
			}
			existingFlowID = ""
		} else {
			log.Info("flow already exists", "alias", flow.Spec.Alias, "id", existingFlowID)
			return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", existingFlowID)
		}
	}

	if existingFlowID == "" {
		// Create flow and execution tree
		log.Info("creating authentication flow", "alias", flow.Spec.Alias, "realm", realmName)
		flowID, err := r.createFlowTree(ctx, kc, realmName, flow)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, flow, false, "CreateFailed", fmt.Sprintf("Failed to create flow: %v", err), "")
		}
		log.Info("authentication flow created", "alias", flow.Spec.Alias, "id", flowID)
		return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", flowID)
	}

	return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", existingFlowID)
}

func (r *KeycloakAuthenticationFlowReconciler) findFlowByAlias(ctx context.Context, kc *keycloak.Client, realmName, alias string) (string, error) {
	flows, err := kc.GetAuthenticationFlows(ctx, realmName)
	if err != nil {
		return "", err
	}
	for _, f := range flows {
		if f.Alias != nil && *f.Alias == alias {
			if f.ID != nil {
				return *f.ID, nil
			}
		}
	}
	return "", nil
}

func (r *KeycloakAuthenticationFlowReconciler) createFlowTree(ctx context.Context, kc *keycloak.Client, realmName string, flow *keycloakv1beta1.KeycloakAuthenticationFlow) (string, error) {
	topLevel := true
	builtIn := false
	flowRep := keycloak.AuthenticationFlowRepresentation{
		Alias:       &flow.Spec.Alias,
		Description: &flow.Spec.Description,
		ProviderID:  &flow.Spec.ProviderId,
		TopLevel:    &topLevel,
		BuiltIn:     &builtIn,
	}

	flowID, err := kc.CreateAuthenticationFlow(ctx, realmName, flowRep)
	if err != nil {
		return "", fmt.Errorf("creating top-level flow %q: %w", flow.Spec.Alias, err)
	}

	if err := r.addExecutions(ctx, kc, realmName, flow.Spec.Alias, flow.Spec.Executions); err != nil {
		// Best-effort cleanup on failure
		_ = kc.DeleteAuthenticationFlow(ctx, realmName, flowID)
		return "", err
	}

	return flowID, nil
}

// addExecutions adds a list of executions to a flow identified by its alias.
// For each sub-flow, it recurses to add the sub-flow's children.
func (r *KeycloakAuthenticationFlowReconciler) addExecutions(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, executions []keycloakv1beta1.AuthenticationExecution) error {
	for _, exec := range executions {
		if exec.SubFlow != nil {
			if err := r.addSubFlow(ctx, kc, realmName, parentAlias, exec); err != nil {
				return err
			}
		} else if exec.Authenticator != "" {
			if err := r.addAuthenticatorExecution(ctx, kc, realmName, parentAlias, exec); err != nil {
				return err
			}
		}
	}

	// Reorder executions to match the spec ordering
	if err := r.reorderExecutions(ctx, kc, realmName, parentAlias, executions); err != nil {
		return fmt.Errorf("reordering executions in flow %q: %w", parentAlias, err)
	}

	return nil
}

func (r *KeycloakAuthenticationFlowReconciler) addAuthenticatorExecution(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, exec keycloakv1beta1.AuthenticationExecution) error {
	_, err := kc.AddFlowExecution(ctx, realmName, parentAlias, exec.Authenticator)
	if err != nil {
		return fmt.Errorf("adding execution %q to flow %q: %w", exec.Authenticator, parentAlias, err)
	}

	// Find the newly added execution to set its requirement
	execInfo, err := r.findExecution(ctx, kc, realmName, parentAlias, exec.Authenticator, false)
	if err != nil {
		return fmt.Errorf("finding execution %q after creation: %w", exec.Authenticator, err)
	}

	if execInfo != nil && (execInfo.Requirement == nil || *execInfo.Requirement != exec.Requirement) {
		execInfo.Requirement = &exec.Requirement
		if err := kc.UpdateFlowExecution(ctx, realmName, parentAlias, *execInfo); err != nil {
			return fmt.Errorf("setting requirement on execution %q: %w", exec.Authenticator, err)
		}
	}

	// Apply authenticator config if specified
	if len(exec.AuthenticatorConfig) > 0 && execInfo != nil && execInfo.ID != nil {
		configAlias := parentAlias + "-" + exec.Authenticator + "-config"
		config := keycloak.AuthenticatorConfigRepresentation{
			Alias:  &configAlias,
			Config: exec.AuthenticatorConfig,
		}
		if _, err := kc.CreateExecutionConfig(ctx, realmName, *execInfo.ID, config); err != nil {
			return fmt.Errorf("setting config on execution %q: %w", exec.Authenticator, err)
		}
	}

	return nil
}

func (r *KeycloakAuthenticationFlowReconciler) addSubFlow(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, exec keycloakv1beta1.AuthenticationExecution) error {
	subFlowDef := map[string]interface{}{
		"alias":       exec.SubFlow.Alias,
		"description": exec.SubFlow.Description,
		"provider":    exec.SubFlow.ProviderId,
		"type":        exec.SubFlow.ProviderId,
	}

	_, err := kc.AddFlowSubFlow(ctx, realmName, parentAlias, subFlowDef)
	if err != nil {
		return fmt.Errorf("adding sub-flow %q to flow %q: %w", exec.SubFlow.Alias, parentAlias, err)
	}

	// Find the newly added sub-flow execution to set its requirement
	execInfo, err := r.findExecution(ctx, kc, realmName, parentAlias, exec.SubFlow.Alias, true)
	if err != nil {
		return fmt.Errorf("finding sub-flow %q after creation: %w", exec.SubFlow.Alias, err)
	}

	if execInfo != nil && (execInfo.Requirement == nil || *execInfo.Requirement != exec.Requirement) {
		execInfo.Requirement = &exec.Requirement
		if err := kc.UpdateFlowExecution(ctx, realmName, parentAlias, *execInfo); err != nil {
			return fmt.Errorf("setting requirement on sub-flow %q: %w", exec.SubFlow.Alias, err)
		}
	}

	if len(exec.SubFlow.Executions) > 0 {
		if err := r.addSubFlowChildren(ctx, kc, realmName, exec.SubFlow.Alias, exec.SubFlow.Executions); err != nil {
			return err
		}
	}

	return nil
}

// addSubFlowChildren adds leaf-level authenticator executions to a sub-flow.
func (r *KeycloakAuthenticationFlowReconciler) addSubFlowChildren(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, executions []keycloakv1beta1.SubFlowExecution) error {
	for _, exec := range executions {
		if _, err := kc.AddFlowExecution(ctx, realmName, parentAlias, exec.Authenticator); err != nil {
			return fmt.Errorf("adding execution %q to sub-flow %q: %w", exec.Authenticator, parentAlias, err)
		}

		execInfo, err := r.findExecution(ctx, kc, realmName, parentAlias, exec.Authenticator, false)
		if err != nil {
			return fmt.Errorf("finding execution %q after creation: %w", exec.Authenticator, err)
		}

		if execInfo != nil && (execInfo.Requirement == nil || *execInfo.Requirement != exec.Requirement) {
			execInfo.Requirement = &exec.Requirement
			if err := kc.UpdateFlowExecution(ctx, realmName, parentAlias, *execInfo); err != nil {
				return fmt.Errorf("setting requirement on execution %q: %w", exec.Authenticator, err)
			}
		}

		if len(exec.AuthenticatorConfig) > 0 && execInfo != nil && execInfo.ID != nil {
			configAlias := parentAlias + "-" + exec.Authenticator + "-config"
			config := keycloak.AuthenticatorConfigRepresentation{
				Alias:  &configAlias,
				Config: exec.AuthenticatorConfig,
			}
			if _, err := kc.CreateExecutionConfig(ctx, realmName, *execInfo.ID, config); err != nil {
				return fmt.Errorf("setting config on execution %q: %w", exec.Authenticator, err)
			}
		}
	}

	if len(executions) > 1 {
		if err := r.reorderSubFlowChildren(ctx, kc, realmName, parentAlias, executions); err != nil {
			return fmt.Errorf("reordering executions in sub-flow %q: %w", parentAlias, err)
		}
	}

	return nil
}

// reorderSubFlowChildren reorders leaf executions within a sub-flow.
func (r *KeycloakAuthenticationFlowReconciler) reorderSubFlowChildren(ctx context.Context, kc *keycloak.Client, realmName, flowAlias string, desiredOrder []keycloakv1beta1.SubFlowExecution) error {
	desired := make([]execIdentifier, 0, len(desiredOrder))
	for _, e := range desiredOrder {
		desired = append(desired, execIdentifier{name: e.Authenticator, isFlow: false})
	}

	for targetIdx := 0; targetIdx < len(desired); targetIdx++ {
		execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		if err != nil {
			return err
		}

		topLevel := filterTopLevelExecutions(execs)

		currentIdx := -1
		for i, e := range topLevel {
			if matchesIdentifier(e, desired[targetIdx]) {
				currentIdx = i
				break
			}
		}

		if currentIdx < 0 || currentIdx == targetIdx {
			continue
		}

		for i := 0; i < currentIdx-targetIdx; i++ {
			if topLevel[currentIdx].ID == nil {
				break
			}
			if err := kc.RaiseExecutionPriority(ctx, realmName, *topLevel[currentIdx].ID); err != nil {
				return fmt.Errorf("raising priority of execution: %w", err)
			}
		}
	}

	return nil
}

// findExecution locates an execution within a flow by its provider ID or alias.
func (r *KeycloakAuthenticationFlowReconciler) findExecution(ctx context.Context, kc *keycloak.Client, realmName, flowAlias, identifier string, isFlow bool) (*keycloak.AuthenticationExecutionInfo, error) {
	execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
	if err != nil {
		return nil, err
	}

	for i := range execs {
		e := &execs[i]
		if isFlow {
			if e.AuthenticationFlow != nil && *e.AuthenticationFlow && e.DisplayName != nil && *e.DisplayName == identifier {
				return e, nil
			}
		} else {
			if e.ProviderID != nil && *e.ProviderID == identifier && (e.AuthenticationFlow == nil || !*e.AuthenticationFlow) {
				return e, nil
			}
		}
	}
	return nil, nil
}

// reorderExecutions reorders executions within a flow to match the spec order.
// Keycloak's API only supports raise/lower-priority, so we use a bubble-sort
// approach: for each desired position, find the execution and raise it until
// it reaches the correct index.
func (r *KeycloakAuthenticationFlowReconciler) reorderExecutions(ctx context.Context, kc *keycloak.Client, realmName, flowAlias string, desiredOrder []keycloakv1beta1.AuthenticationExecution) error {
	if len(desiredOrder) <= 1 {
		return nil
	}

	desired := make([]execIdentifier, 0, len(desiredOrder))
	for _, e := range desiredOrder {
		if e.SubFlow != nil {
			desired = append(desired, execIdentifier{name: e.SubFlow.Alias, isFlow: true})
		} else {
			desired = append(desired, execIdentifier{name: e.Authenticator, isFlow: false})
		}
	}

	// Repeatedly fetch current state and bubble executions into position
	for targetIdx := 0; targetIdx < len(desired); targetIdx++ {
		execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
		if err != nil {
			return err
		}

		// Filter to only top-level executions (level 0) for this flow
		topLevel := filterTopLevelExecutions(execs)

		currentIdx := -1
		for i, e := range topLevel {
			if matchesIdentifier(e, desired[targetIdx]) {
				currentIdx = i
				break
			}
		}

		if currentIdx < 0 || currentIdx == targetIdx {
			continue
		}

		// Raise priority (currentIdx - targetIdx) times to move it up
		for i := 0; i < currentIdx-targetIdx; i++ {
			if topLevel[currentIdx].ID == nil {
				break
			}
			if err := kc.RaiseExecutionPriority(ctx, realmName, *topLevel[currentIdx].ID); err != nil {
				return fmt.Errorf("raising priority of execution: %w", err)
			}
		}
	}

	return nil
}

// filterTopLevelExecutions returns only executions at level 0 (direct children of the flow).
func filterTopLevelExecutions(execs []keycloak.AuthenticationExecutionInfo) []keycloak.AuthenticationExecutionInfo {
	var result []keycloak.AuthenticationExecutionInfo
	for _, e := range execs {
		if e.Level != nil && *e.Level == 0 {
			result = append(result, e)
		}
	}
	// If no level info, return all (Keycloak versions may differ)
	if len(result) == 0 {
		return execs
	}
	return result
}

type execIdentifier struct {
	name   string
	isFlow bool
}

func matchesIdentifier(e keycloak.AuthenticationExecutionInfo, id execIdentifier) bool {
	if id.isFlow {
		return e.AuthenticationFlow != nil && *e.AuthenticationFlow && e.DisplayName != nil && *e.DisplayName == id.name
	}
	return e.ProviderID != nil && *e.ProviderID == id.name && (e.AuthenticationFlow == nil || !*e.AuthenticationFlow)
}

func (r *KeycloakAuthenticationFlowReconciler) deleteFlow(ctx context.Context, flow *keycloakv1beta1.KeycloakAuthenticationFlow) error {
	if flow.Status.FlowID == "" {
		return nil
	}

	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, flow)
	if err != nil {
		return err
	}

	return kc.DeleteAuthenticationFlow(ctx, realmName, flow.Status.FlowID)
}

func (r *KeycloakAuthenticationFlowReconciler) getKeycloakClientAndRealm(ctx context.Context, flow *keycloakv1beta1.KeycloakAuthenticationFlow) (*keycloak.Client, string, error) {
	if flow.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, flow.Spec.ClusterRealmRef.Name)
	}

	if flow.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmNamespace := flow.Namespace
	if flow.Spec.RealmRef.Namespace != nil {
		realmNamespace = *flow.Spec.RealmRef.Namespace
	}
	realmKey := types.NamespacedName{
		Name:      flow.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmKey, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmKey, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmKey)
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}
	realmName := realmDef.Realm

	if realm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("realm %s has no instanceRef", realmKey)
	}

	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceKey := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceKey, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceKey, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceKey)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceKey.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceKey)
	}

	return kc, realmName, nil
}

func (r *KeycloakAuthenticationFlowReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
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

	instanceKey := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceKey, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceKey, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceKey)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceKey.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceKey)
	}

	return kc, realmName, nil
}

func (r *KeycloakAuthenticationFlowReconciler) updateStatus(ctx context.Context, flow *keycloakv1beta1.KeycloakAuthenticationFlow, ready bool, status, message, flowID string) (ctrl.Result, error) {
	flow.Status.Ready = ready
	flow.Status.Status = status
	flow.Status.Message = message
	flow.Status.FlowID = flowID
	if flowID != "" {
		flow.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/authentication/flows/%s", flow.Spec.Alias, flowID)
	}

	if ready {
		flow.Status.ObservedGeneration = flow.Generation
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
	for i, c := range flow.Status.Conditions {
		if c.Type == "Ready" {
			flow.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		flow.Status.Conditions = append(flow.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, flow); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakAuthenticationFlowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakAuthenticationFlow{}).
		Complete(r)
}
