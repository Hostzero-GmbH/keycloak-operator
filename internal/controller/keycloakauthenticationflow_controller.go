package controller

import (
	"context"
	"encoding/json"
	"fmt"
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

// flowDefinition is the recursive representation of a (sub-)flow that the
// controller works with after decoding the spec's free-form executions field.
type flowDefinition struct {
	Alias       string          `json:"alias"`
	Description string          `json:"description,omitempty"`
	ProviderID  string          `json:"providerId"`
	Executions  []flowExecution `json:"executions,omitempty"`
}

// flowExecution is one node in the execution tree. Exactly one of
// Authenticator or SubFlow must be set per node.
type flowExecution struct {
	Authenticator       string            `json:"authenticator,omitempty"`
	SubFlow             *flowDefinition   `json:"subFlow,omitempty"`
	Requirement         string            `json:"requirement"`
	AuthenticatorConfig map[string]string `json:"authenticatorConfig,omitempty"`
	// Executions accepts the "sibling" YAML shape, where child executions
	// live next to subFlow rather than inside it. Both shapes are merged in
	// declaration order: inline children (subFlow.executions) first, then
	// sibling children (this field).
	Executions []flowExecution `json:"executions,omitempty"`
}

// children returns the merged list of child executions for a sub-flow node:
// inline children inside subFlow.executions first, then sibling children
// declared next to subFlow. Returns nil for leaf authenticator nodes.
func (e flowExecution) children() []flowExecution {
	if e.SubFlow == nil {
		return nil
	}
	if len(e.Executions) == 0 {
		return e.SubFlow.Executions
	}
	if len(e.SubFlow.Executions) == 0 {
		return e.Executions
	}
	merged := make([]flowExecution, 0, len(e.SubFlow.Executions)+len(e.Executions))
	merged = append(merged, e.SubFlow.Executions...)
	merged = append(merged, e.Executions...)
	return merged
}

// parseExecutions decodes the spec's executions field into the recursive
// representation. Validation errors include the JSON-pointer-style path of
// the offending node, e.g. "[1].executions[0].requirement is required".
func parseExecutions(raw runtime.RawExtension) ([]flowExecution, error) {
	if len(raw.Raw) == 0 {
		return nil, nil
	}
	var execs []flowExecution
	if err := json.Unmarshal(raw.Raw, &execs); err != nil {
		return nil, fmt.Errorf("decoding executions: %w", err)
	}
	if err := validateExecutions(execs, ""); err != nil {
		return nil, err
	}
	return execs, nil
}

func validateExecutions(execs []flowExecution, path string) error {
	for i, e := range execs {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		hasAuth := e.Authenticator != ""
		hasSub := e.SubFlow != nil
		if hasAuth == hasSub {
			return fmt.Errorf("%s: exactly one of authenticator or subFlow must be set", nodePath)
		}
		if hasSub {
			if strings.TrimSpace(e.SubFlow.Alias) == "" {
				return fmt.Errorf("%s.subFlow.alias is required", nodePath)
			}
			if strings.TrimSpace(e.SubFlow.ProviderID) == "" {
				return fmt.Errorf("%s.subFlow.providerId is required", nodePath)
			}
		}
		if e.Requirement == "" {
			return fmt.Errorf("%s.requirement is required", nodePath)
		}
		switch e.Requirement {
		case "REQUIRED", "ALTERNATIVE", "DISABLED", "CONDITIONAL":
		default:
			return fmt.Errorf("%s.requirement %q is not one of REQUIRED|ALTERNATIVE|DISABLED|CONDITIONAL", nodePath, e.Requirement)
		}
		if hasSub {
			if err := validateExecutions(e.children(), nodePath+".executions"); err != nil {
				return err
			}
		}
	}
	return nil
}

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
		return r.updateStatus(ctx, flow, false, "RealmNotReady", err.Error(), "", "")
	}

	// Validate the spec early so we report decoding/shape errors with a clear
	// message instead of failing later inside a Keycloak API call.
	executions, err := parseExecutions(flow.Spec.Executions)
	if err != nil {
		RecordError(controllerName, "invalid_spec")
		return r.updateStatus(ctx, flow, false, "InvalidSpec", err.Error(), "", realmName)
	}

	// Find existing flow by alias
	existingFlowID, err := r.findFlowByAlias(ctx, kc, realmName, flow.Spec.Alias)
	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, flow, false, "APIError", fmt.Sprintf("Failed to list flows: %v", err), "", realmName)
	}

	if existingFlowID != "" {
		// Phase 1: on spec change, delete and recreate
		if flow.Status.ObservedGeneration != 0 && flow.Status.ObservedGeneration < flow.Generation {
			log.Info("spec changed, recreating flow", "alias", flow.Spec.Alias)
			if err := kc.DeleteAuthenticationFlow(ctx, realmName, existingFlowID); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, flow, false, "DeleteFailed", fmt.Sprintf("Failed to delete flow for recreation: %v", err), existingFlowID, realmName)
			}
		} else {
			log.Info("flow already exists", "alias", flow.Spec.Alias, "id", existingFlowID)
			return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", existingFlowID, realmName)
		}
	}

	// Create flow and execution tree
	log.Info("creating authentication flow", "alias", flow.Spec.Alias, "realm", realmName)
	flowID, err := r.createFlowTree(ctx, kc, realmName, flow, executions)
	if err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, flow, false, "CreateFailed", fmt.Sprintf("Failed to create flow: %v", err), "", realmName)
	}
	log.Info("authentication flow created", "alias", flow.Spec.Alias, "id", flowID)
	return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", flowID, realmName)
}

func (r *KeycloakAuthenticationFlowReconciler) findFlowByAlias(ctx context.Context, kc *keycloak.Client, realmName, alias string) (string, error) {
	flows, err := kc.GetAuthenticationFlows(ctx, realmName)
	if err != nil {
		return "", err
	}
	for _, f := range flows {
		if f.Alias != nil && *f.Alias == alias && f.ID != nil {
			return *f.ID, nil
		}
	}
	return "", nil
}

func (r *KeycloakAuthenticationFlowReconciler) createFlowTree(ctx context.Context, kc *keycloak.Client, realmName string, flow *keycloakv1beta1.KeycloakAuthenticationFlow, executions []flowExecution) (string, error) {
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

	if err := r.addExecutions(ctx, kc, realmName, flow.Spec.Alias, executions); err != nil {
		// Best-effort cleanup on failure
		_ = kc.DeleteAuthenticationFlow(ctx, realmName, flowID)
		return "", err
	}

	return flowID, nil
}

// addExecutions adds an ordered list of executions under parentAlias and
// recurses into any sub-flows. Reordering is performed once per parent at the
// end so each new node sits at the correct position regardless of the order
// Keycloak assigns to newly added executions.
func (r *KeycloakAuthenticationFlowReconciler) addExecutions(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, executions []flowExecution) error {
	for _, exec := range executions {
		if exec.SubFlow != nil {
			if err := r.addSubFlow(ctx, kc, realmName, parentAlias, exec); err != nil {
				return err
			}
		} else {
			if err := r.addAuthenticatorExecution(ctx, kc, realmName, parentAlias, exec); err != nil {
				return err
			}
		}
	}
	if len(executions) > 1 {
		if err := r.reorderChildren(ctx, kc, realmName, parentAlias, executions); err != nil {
			return fmt.Errorf("reordering executions in flow %q: %w", parentAlias, err)
		}
	}
	return nil
}

func (r *KeycloakAuthenticationFlowReconciler) addAuthenticatorExecution(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, exec flowExecution) error {
	if _, err := kc.AddFlowExecution(ctx, realmName, parentAlias, exec.Authenticator); err != nil {
		return fmt.Errorf("adding execution %q to flow %q: %w", exec.Authenticator, parentAlias, err)
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

	return nil
}

func (r *KeycloakAuthenticationFlowReconciler) addSubFlow(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, exec flowExecution) error {
	subFlowDef := buildSubFlowDef(exec.SubFlow.Alias, exec.SubFlow.Description, exec.SubFlow.ProviderID)
	if _, err := kc.AddFlowSubFlow(ctx, realmName, parentAlias, subFlowDef); err != nil {
		return fmt.Errorf("adding sub-flow %q to flow %q: %w", exec.SubFlow.Alias, parentAlias, err)
	}

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

	children := exec.children()
	if len(children) > 0 {
		if err := r.addExecutions(ctx, kc, realmName, exec.SubFlow.Alias, children); err != nil {
			return err
		}
	}
	return nil
}

// buildSubFlowDef constructs the request body for adding a sub-flow execution.
// Empty optional fields are omitted to avoid unintentionally clearing values
// on Keycloak versions that distinguish between absent and empty strings.
func buildSubFlowDef(alias, description, providerId string) map[string]interface{} {
	def := map[string]interface{}{
		"alias":    alias,
		"provider": providerId,
		"type":     providerId,
	}
	if description != "" {
		def["description"] = description
	}
	return def
}

// reorderChildren bubble-sorts the direct children of parentAlias into the
// order described by desired, using Keycloak's raise-priority endpoint (the
// only tool the API offers for reordering).
func (r *KeycloakAuthenticationFlowReconciler) reorderChildren(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, desired []flowExecution) error {
	ids := make([]execIdentifier, 0, len(desired))
	for _, e := range desired {
		if e.SubFlow != nil {
			ids = append(ids, execIdentifier{name: e.SubFlow.Alias, isFlow: true})
		} else {
			ids = append(ids, execIdentifier{name: e.Authenticator, isFlow: false})
		}
	}

	for targetIdx := 0; targetIdx < len(ids); targetIdx++ {
		execs, err := kc.GetFlowExecutions(ctx, realmName, parentAlias)
		if err != nil {
			return err
		}
		topLevel := filterTopLevelExecutions(execs)

		currentIdx := -1
		for i, e := range topLevel {
			if matchesIdentifier(e, ids[targetIdx]) {
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

// findExecution locates a direct child of flowAlias by its provider ID
// (authenticators) or display name (sub-flows).
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

// filterTopLevelExecutions returns only executions at level 0 (direct children
// of the queried flow). When the Keycloak version omits level info we fall
// back to returning everything the API gave us.
func filterTopLevelExecutions(execs []keycloak.AuthenticationExecutionInfo) []keycloak.AuthenticationExecutionInfo {
	var result []keycloak.AuthenticationExecutionInfo
	for _, e := range execs {
		if e.Level != nil && *e.Level == 0 {
			result = append(result, e)
		}
	}
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

func (r *KeycloakAuthenticationFlowReconciler) updateStatus(ctx context.Context, flow *keycloakv1beta1.KeycloakAuthenticationFlow, ready bool, status, message, flowID, realmName string) (ctrl.Result, error) {
	flow.Status.Ready = ready
	flow.Status.Status = status
	flow.Status.Message = message
	flow.Status.FlowID = flowID
	if flowID != "" && realmName != "" {
		flow.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/authentication/flows/%s", realmName, flowID)
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
