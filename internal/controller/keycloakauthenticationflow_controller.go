package controller

import (
	"context"
	"encoding/json"
	stderrors "errors"
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

// errProviderChangeUnsupported is returned when a spec change requests a
// different top-level providerId than the flow that already exists in
// Keycloak. Switching the top-level provider type (e.g. basic-flow ->
// client-flow) is not supported by the Keycloak Admin API; users must pick a
// new alias instead.
var errProviderChangeUnsupported = stderrors.New("authentication flow provider change is not supported")

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
		stats, err := r.updateExistingFlow(ctx, kc, realmName, flow, existingFlowID, executions)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			if stderrors.Is(err, errProviderChangeUnsupported) {
				return r.updateStatus(ctx, flow, false, "ProviderChangeUnsupported", err.Error(), existingFlowID, realmName)
			}
			return r.updateStatus(ctx, flow, false, "UpdateFailed", fmt.Sprintf("Failed to update flow: %v", err), existingFlowID, realmName)
		}
		if stats.touched() {
			log.Info("authentication flow updated in place",
				"alias", flow.Spec.Alias,
				"id", existingFlowID,
				"added", stats.added,
				"updated", stats.updated,
				"removed", stats.removed,
				"reorderedParents", stats.reorderedParents,
			)
		} else {
			log.V(1).Info("flow already in sync, skipping update", "alias", flow.Spec.Alias, "id", existingFlowID)
		}
		return r.updateStatus(ctx, flow, true, "Ready", "Authentication flow synchronized", existingFlowID, realmName)
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
		if _, err := r.reorderChildren(ctx, kc, realmName, parentAlias, executions); err != nil {
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

// Gap left between priorities so future inserts don't need to renumber.
const reorderPriorityStep = 10

// reorderChildren PUTs an explicit priority on each child of parentAlias so
// the live order matches desired. Requires Keycloak 25+: older versions drop
// the priority field, and the raise-/lower-priority swap endpoints don't
// help on KC 25/26 either because new executions all share priority 0
// (keycloak/keycloak#35765). Returns true when at least one PUT was issued.
func (r *KeycloakAuthenticationFlowReconciler) reorderChildren(ctx context.Context, kc *keycloak.Client, realmName, parentAlias string, desired []flowExecution) (bool, error) {
	execs, err := kc.GetFlowExecutions(ctx, realmName, parentAlias)
	if err != nil {
		return false, err
	}
	topLevel := filterTopLevelExecutions(execs)

	matchIdx := make([]int, len(desired))
	used := make([]bool, len(topLevel))
	for i, d := range desired {
		id := desiredIdentifier(d)
		matchIdx[i] = -1
		for j := range topLevel {
			if used[j] {
				continue
			}
			if matchesIdentifier(topLevel[j], id) {
				matchIdx[i] = j
				used[j] = true
				break
			}
		}
		if matchIdx[i] < 0 {
			return false, fmt.Errorf("execution %q not found in flow %q after add (cannot reorder)", id.name, parentAlias)
		}
	}

	// Position equality alone is not enough: with all priorities at 0,
	// Keycloak's sort order for ties is non-deterministic, so we'd skip
	// the PUTs and never lock the order in. Require a positive priority.
	if liveMatchesDesiredOrder(topLevel, matchIdx) {
		return false, nil
	}

	for i, j := range matchIdx {
		match := topLevel[j]
		if match.ID == nil {
			continue
		}
		priority := (i + 1) * reorderPriorityStep
		upd := keycloak.AuthenticationExecutionInfo{
			ID:       match.ID,
			Priority: &priority,
		}
		if match.Requirement != nil {
			upd.Requirement = match.Requirement
		}
		if match.AuthenticationFlow != nil {
			upd.AuthenticationFlow = match.AuthenticationFlow
		}
		if err := kc.UpdateFlowExecution(ctx, realmName, parentAlias, upd); err != nil {
			return true, fmt.Errorf("setting priority on execution %q in flow %q: %w", desired[i].Authenticator, parentAlias, err)
		}
	}
	return true, nil
}

func liveMatchesDesiredOrder(topLevel []keycloak.AuthenticationExecutionInfo, matchIdx []int) bool {
	for i, j := range matchIdx {
		if i != j {
			return false
		}
		if topLevel[j].Priority == nil || *topLevel[j].Priority <= 0 {
			return false
		}
	}
	return true
}

func desiredIdentifier(e flowExecution) execIdentifier {
	if e.SubFlow != nil {
		return execIdentifier{name: e.SubFlow.Alias, isFlow: true}
	}
	return execIdentifier{name: e.Authenticator, isFlow: false}
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

// liveExecution mirrors flowExecution but carries the IDs and metadata
// required to mutate the live tree (Keycloak execution UUID, config UUID).
// It is the result of walking the live flow with readLiveTree.
type liveExecution struct {
	ID                   string
	Requirement          string
	AuthenticationConfig string
	IsFlow               bool
	Authenticator        string
	SubFlowAlias         string
	Children             []liveExecution
}

// readLiveTree fetches the live execution tree under flowAlias and returns it
// in a comparable shape so reconcileChildren can diff it against the spec.
func (r *KeycloakAuthenticationFlowReconciler) readLiveTree(ctx context.Context, kc *keycloak.Client, realmName, flowAlias string) ([]liveExecution, error) {
	execs, err := kc.GetFlowExecutions(ctx, realmName, flowAlias)
	if err != nil {
		return nil, err
	}
	children := filterTopLevelExecutions(execs)
	out := make([]liveExecution, 0, len(children))
	for _, e := range children {
		le := liveExecution{}
		if e.ID != nil {
			le.ID = *e.ID
		}
		if e.Requirement != nil {
			le.Requirement = *e.Requirement
		}
		if e.AuthenticationConfig != nil {
			le.AuthenticationConfig = *e.AuthenticationConfig
		}
		if e.AuthenticationFlow != nil && *e.AuthenticationFlow {
			le.IsFlow = true
			if e.DisplayName != nil {
				le.SubFlowAlias = *e.DisplayName
			}
			if le.SubFlowAlias != "" {
				kids, err := r.readLiveTree(ctx, kc, realmName, le.SubFlowAlias)
				if err != nil {
					return nil, err
				}
				le.Children = kids
			}
		} else if e.ProviderID != nil {
			le.Authenticator = *e.ProviderID
		}
		out = append(out, le)
	}
	return out, nil
}

// updateStats is a small counter aggregate used to log a one-line summary at
// the end of a reconcile, so users can see what actually changed.
type updateStats struct {
	added, updated, removed, reorderedParents int
}

func (s *updateStats) touched() bool {
	if s == nil {
		return false
	}
	return s.added > 0 || s.updated > 0 || s.removed > 0 || s.reorderedParents > 0
}

// matchExecutions pairs each desired entry with at most one live entry of the
// same identity (provider id for leaves, alias for sub-flows). Matching is
// occurrence-based: the i-th desired entry with identity X matches the i-th
// unmatched live entry with the same identity. Unmatched entries on either
// side surface as adds (desired) and removes (live). Returned matches[i] is
// the live index for desired[i], or -1 if desired[i] has no match.
func matchExecutions(desired []flowExecution, live []liveExecution) (matches []int, matchedLive []bool) {
	matches = make([]int, len(desired))
	for i := range matches {
		matches[i] = -1
	}
	matchedLive = make([]bool, len(live))
	for di, d := range desired {
		desiredIsFlow := d.SubFlow != nil
		for li, l := range live {
			if matchedLive[li] || l.IsFlow != desiredIsFlow {
				continue
			}
			if desiredIsFlow {
				if d.SubFlow != nil && d.SubFlow.Alias == l.SubFlowAlias {
					matches[di] = li
					matchedLive[li] = true
					break
				}
			} else if d.Authenticator == l.Authenticator {
				matches[di] = li
				matchedLive[li] = true
				break
			}
		}
	}
	return matches, matchedLive
}

// reconcileChildren brings the live children of parentAlias into the shape
// described by desired with the minimum set of API calls (Add / Delete /
// Update / Reorder), recursing into matched sub-flows. The top-level flow
// itself is never deleted from this path, so flows that are referenced as a
// sub-flow execution by another flow or as a realm binding override stay
// usable throughout the update.
func (r *KeycloakAuthenticationFlowReconciler) reconcileChildren(
	ctx context.Context, kc *keycloak.Client, realmName, parentAlias string,
	desired []flowExecution, live []liveExecution, stats *updateStats,
) error {
	matches, matchedLive := matchExecutions(desired, live)

	for li, l := range live {
		if matchedLive[li] || l.ID == "" {
			continue
		}
		if err := kc.DeleteExecution(ctx, realmName, l.ID); err != nil {
			return fmt.Errorf("removing %s from flow %q: %w", liveIdentity(l), parentAlias, err)
		}
		stats.removed++
	}

	for di, d := range desired {
		li := matches[di]
		if li < 0 {
			continue
		}
		l := live[li]
		if l.Requirement != d.Requirement {
			if err := r.setExecutionRequirement(ctx, kc, realmName, parentAlias, l.ID, d.Requirement, l.IsFlow); err != nil {
				return err
			}
			stats.updated++
		}
		if l.IsFlow {
			if err := r.reconcileChildren(ctx, kc, realmName, d.SubFlow.Alias, d.children(), l.Children, stats); err != nil {
				return err
			}
			continue
		}
		if err := r.reconcileLeafConfig(ctx, kc, realmName, parentAlias, l, d, stats); err != nil {
			return err
		}
	}

	for di, d := range desired {
		if matches[di] >= 0 {
			continue
		}
		if d.SubFlow != nil {
			if err := r.addSubFlow(ctx, kc, realmName, parentAlias, d); err != nil {
				return err
			}
		} else {
			if err := r.addAuthenticatorExecution(ctx, kc, realmName, parentAlias, d); err != nil {
				return err
			}
		}
		stats.added++
	}

	if len(desired) > 1 {
		changed, err := r.reorderChildren(ctx, kc, realmName, parentAlias, desired)
		if err != nil {
			return fmt.Errorf("reordering executions in flow %q: %w", parentAlias, err)
		}
		if changed {
			stats.reorderedParents++
		}
	}
	return nil
}

// setExecutionRequirement updates an existing execution's requirement via the
// flow-scoped PUT endpoint. Only ID and Requirement need to be populated;
// Keycloak ignores the rest of the body.
func (r *KeycloakAuthenticationFlowReconciler) setExecutionRequirement(
	ctx context.Context, kc *keycloak.Client, realmName, parentAlias, executionID, requirement string, isFlow bool,
) error {
	info := keycloak.AuthenticationExecutionInfo{
		ID:          &executionID,
		Requirement: &requirement,
	}
	if isFlow {
		t := true
		info.AuthenticationFlow = &t
	}
	if err := kc.UpdateFlowExecution(ctx, realmName, parentAlias, info); err != nil {
		return fmt.Errorf("setting requirement on execution in flow %q: %w", parentAlias, err)
	}
	return nil
}

// reconcileLeafConfig converges the authenticatorConfig of a matched leaf
// execution. Combinations handled: none/none -> no-op; spec/none -> create;
// none/live -> delete; spec/live equal -> no-op; spec/live differ -> update.
func (r *KeycloakAuthenticationFlowReconciler) reconcileLeafConfig(
	ctx context.Context, kc *keycloak.Client, realmName, parentAlias string,
	l liveExecution, d flowExecution, stats *updateStats,
) error {
	hasDesired := len(d.AuthenticatorConfig) > 0
	hasLive := l.AuthenticationConfig != ""

	switch {
	case !hasDesired && !hasLive:
		return nil
	case hasDesired && !hasLive:
		configAlias := parentAlias + "-" + d.Authenticator + "-config"
		config := keycloak.AuthenticatorConfigRepresentation{
			Alias:  &configAlias,
			Config: d.AuthenticatorConfig,
		}
		if _, err := kc.CreateExecutionConfig(ctx, realmName, l.ID, config); err != nil {
			return fmt.Errorf("setting config on execution %q in flow %q: %w", d.Authenticator, parentAlias, err)
		}
		stats.updated++
		return nil
	case !hasDesired && hasLive:
		if err := kc.DeleteExecutionConfig(ctx, realmName, l.AuthenticationConfig); err != nil {
			return fmt.Errorf("removing config from execution %q in flow %q: %w", d.Authenticator, parentAlias, err)
		}
		stats.updated++
		return nil
	default:
		liveCfg, err := kc.GetExecutionConfig(ctx, realmName, l.AuthenticationConfig)
		if err != nil {
			return fmt.Errorf("fetching live config for execution %q: %w", d.Authenticator, err)
		}
		if configMapsEqual(liveCfg.Config, d.AuthenticatorConfig) {
			return nil
		}
		configAlias := parentAlias + "-" + d.Authenticator + "-config"
		if liveCfg.Alias != nil && *liveCfg.Alias != "" {
			configAlias = *liveCfg.Alias
		}
		update := keycloak.AuthenticatorConfigRepresentation{
			ID:     &l.AuthenticationConfig,
			Alias:  &configAlias,
			Config: d.AuthenticatorConfig,
		}
		if err := kc.UpdateExecutionConfig(ctx, realmName, l.AuthenticationConfig, update); err != nil {
			return fmt.Errorf("updating config on execution %q in flow %q: %w", d.Authenticator, parentAlias, err)
		}
		stats.updated++
		return nil
	}
}

func configMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// liveIdentity returns a short human-readable identifier for a live execution
// for use in log and error messages.
func liveIdentity(l liveExecution) string {
	if l.IsFlow {
		return fmt.Sprintf("sub-flow %q", l.SubFlowAlias)
	}
	return fmt.Sprintf("authenticator %q", l.Authenticator)
}

// updateExistingFlow converges a flow that already exists in Keycloak with
// the desired spec without deleting and recreating it. This path keeps the
// top-level flow ID stable, which is necessary for flows referenced as a
// sub-flow execution by another flow or as a realm binding override.
func (r *KeycloakAuthenticationFlowReconciler) updateExistingFlow(
	ctx context.Context, kc *keycloak.Client, realmName string,
	flow *keycloakv1beta1.KeycloakAuthenticationFlow, existingFlowID string, executions []flowExecution,
) (*updateStats, error) {
	flows, err := kc.GetAuthenticationFlows(ctx, realmName)
	if err != nil {
		return nil, fmt.Errorf("fetching live flow %q: %w", flow.Spec.Alias, err)
	}
	var live *keycloak.AuthenticationFlowRepresentation
	for i := range flows {
		if flows[i].ID != nil && *flows[i].ID == existingFlowID {
			live = &flows[i]
			break
		}
	}
	if live == nil {
		return nil, fmt.Errorf("flow %q (%s) not found in realm %s", flow.Spec.Alias, existingFlowID, realmName)
	}

	if live.ProviderID != nil && *live.ProviderID != flow.Spec.ProviderId {
		return nil, fmt.Errorf("%w: top-level flow provider %q cannot be changed to %q (delete the resource and create a new one with a different alias)",
			errProviderChangeUnsupported, *live.ProviderID, flow.Spec.ProviderId)
	}

	stats := &updateStats{}

	liveDescription := ""
	if live.Description != nil {
		liveDescription = *live.Description
	}
	if liveDescription != flow.Spec.Description {
		topLevel := true
		builtIn := false
		upd := keycloak.AuthenticationFlowRepresentation{
			ID:          &existingFlowID,
			Alias:       &flow.Spec.Alias,
			Description: &flow.Spec.Description,
			ProviderID:  &flow.Spec.ProviderId,
			TopLevel:    &topLevel,
			BuiltIn:     &builtIn,
		}
		if err := kc.UpdateAuthenticationFlow(ctx, realmName, existingFlowID, upd); err != nil {
			return nil, fmt.Errorf("updating top-level fields of flow %q: %w", flow.Spec.Alias, err)
		}
		stats.updated++
	}

	liveTree, err := r.readLiveTree(ctx, kc, realmName, flow.Spec.Alias)
	if err != nil {
		return nil, fmt.Errorf("reading live execution tree for flow %q: %w", flow.Spec.Alias, err)
	}

	if err := r.reconcileChildren(ctx, kc, realmName, flow.Spec.Alias, executions, liveTree, stats); err != nil {
		return nil, err
	}
	return stats, nil
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

	realmKey := types.NamespacedName{
		Name:      flow.Spec.RealmRef.Name,
		Namespace: flow.Namespace,
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

	instanceKey := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: realm.Namespace,
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
