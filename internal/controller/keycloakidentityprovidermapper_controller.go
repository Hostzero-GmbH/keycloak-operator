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

// KeycloakIdentityProviderMapperReconciler reconciles a KeycloakIdentityProviderMapper object
type KeycloakIdentityProviderMapperReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityprovidermappers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityprovidermappers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityprovidermappers/finalizers,verbs=update

// Reconcile handles KeycloakIdentityProviderMapper reconciliation
func (r *KeycloakIdentityProviderMapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakIdentityProviderMapper"

	mapper := &keycloakv1beta1.KeycloakIdentityProviderMapper{}
	if err := r.Get(ctx, req.NamespacedName, mapper); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakIdentityProviderMapper")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	defer func() {
		RecordReconcile(controllerName, mapper.Status.Ready, time.Since(startTime).Seconds())
	}()

	if !mapper.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mapper, FinalizerName) {
			if ShouldPreserveResource(mapper) {
				log.Info("preserving identity provider mapper in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteMapper(ctx, mapper); err != nil {
				log.Error(err, "failed to delete identity provider mapper from Keycloak")
			}

			controllerutil.RemoveFinalizer(mapper, FinalizerName)
			if err := r.Update(ctx, mapper); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(mapper, FinalizerName) {
		controllerutil.AddFinalizer(mapper, FinalizerName)
		if err := r.Update(ctx, mapper); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	kc, realmName, alias, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		RecordError(controllerName, "parent_not_ready")
		return r.updateStatus(ctx, mapper, false, "ParentNotReady", err.Error(), "", "", "")
	}

	var mapperDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(mapper.Spec.Definition.Raw, &mapperDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, mapper, false, "InvalidDefinition", fmt.Sprintf("Failed to parse mapper definition: %v", err), "", "", alias)
	}

	mapperName := mapperDef.Name
	if mapperName == "" {
		mapperName = mapper.Name
	}

	definition := setFieldInDefinition(mapper.Spec.Definition.Raw, "name", mapperName)
	definition = setFieldInDefinition(definition, "identityProviderAlias", alias)

	// Keep the full representation around to drift-compare below — avoids a
	// second GET round-trip per mapper per reconcile.
	var (
		mapperID       string
		existingMapper *keycloak.IdentityProviderMapperRepresentation
	)
	existingMappers, err := kc.GetIdentityProviderMappers(ctx, realmName, alias)
	if err == nil {
		for i := range existingMappers {
			if existingMappers[i].Name != nil && *existingMappers[i].Name == mapperName {
				if existingMappers[i].ID != nil {
					mapperID = *existingMappers[i].ID
				}
				existingMapper = &existingMappers[i]
				break
			}
		}
	}

	if mapperID == "" {
		log.Info("creating identity provider mapper", "name", mapperName, "realm", realmName, "alias", alias)
		mapperID, err = kc.CreateIdentityProviderMapper(ctx, realmName, alias, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, mapper, false, "CreateFailed", fmt.Sprintf("Failed to create identity provider mapper: %v", err), "", "", alias)
		}
		log.Info("identity provider mapper created successfully", "name", mapperName, "id", mapperID)
	} else {
		drifted, compareErr := identityProviderMapperDrifted(definition, existingMapper)
		if compareErr != nil {
			log.Error(compareErr, "failed to compare current mapper state, falling through to update")
		}
		if drifted {
			definition = mergeIDIntoDefinition(definition, &mapperID)
			log.Info("updating identity provider mapper", "name", mapperName, "realm", realmName, "alias", alias)
			if err := kc.UpdateIdentityProviderMapper(ctx, realmName, alias, mapperID, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, mapper, false, "UpdateFailed", fmt.Sprintf("Failed to update identity provider mapper: %v", err), mapperID, mapperName, alias)
			}
			log.Info("identity provider mapper updated successfully", "name", mapperName)
		} else {
			log.V(1).Info("identity provider mapper already in sync, skipping update", "name", mapperName)
		}
	}

	mapper.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers/%s", realmName, alias, mapperID)
	return r.updateStatus(ctx, mapper, true, "Ready", "Identity provider mapper synchronized", mapperID, mapperName, alias)
}

// identityProviderMapperDrifted compares the desired mapper definition against
// the representation Keycloak returned from GET. Returns true when the
// reconciler should issue a PUT (the safe default: drifted=true also covers
// "we couldn't decide" cases — caller logs the error and updates anyway).
//
// Decision matrix:
//   - current == nil: no representation to compare → drifted (force update).
//   - marshal of current fails: drifted + error (caller logs, updates anyway).
//   - definitionsMatch returns true: in sync → not drifted.
//   - otherwise: drifted.
func identityProviderMapperDrifted(desired json.RawMessage, current *keycloak.IdentityProviderMapperRepresentation) (bool, error) {
	if current == nil {
		return true, nil
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		return true, fmt.Errorf("marshal current mapper state: %w", err)
	}
	if definitionsMatch(desired, currentJSON) {
		return false, nil
	}
	return true, nil
}

// getKeycloakClientAndParent loads the parent KeycloakIdentityProvider, ensures
// it is Ready, and resolves the Keycloak admin client, realm name, and IdP
// alias used to address mappers under that IdP.
func (r *KeycloakIdentityProviderMapperReconciler) getKeycloakClientAndParent(ctx context.Context, mapper *keycloakv1beta1.KeycloakIdentityProviderMapper) (*keycloak.Client, string, string, error) {
	idpKey := types.NamespacedName{
		Name:      mapper.Spec.IdentityProviderRef.Name,
		Namespace: mapper.Namespace,
	}

	idp := &keycloakv1beta1.KeycloakIdentityProvider{}
	if err := r.Get(ctx, idpKey, idp); err != nil {
		return nil, "", "", fmt.Errorf("failed to get KeycloakIdentityProvider %s: %w", idpKey, err)
	}

	if !idp.Status.Ready {
		return nil, "", "", fmt.Errorf("KeycloakIdentityProvider %s is not ready", idpKey)
	}

	alias, err := identityProviderAlias(idp)
	if err != nil {
		return nil, "", "", err
	}

	kc, realmName, err := GetKeycloakClientAndRealmForIDP(ctx, r.Client, r.ClientManager, idp)
	if err != nil {
		return nil, "", "", err
	}

	return kc, realmName, alias, nil
}

// identityProviderAlias extracts the Keycloak alias for the given
// KeycloakIdentityProvider, following the same fallback as the IdP controller:
// the alias from spec.definition, falling back to the CR's metadata.name.
func identityProviderAlias(idp *keycloakv1beta1.KeycloakIdentityProvider) (string, error) {
	var idpDef struct {
		Alias string `json:"alias"`
	}
	if len(idp.Spec.Definition.Raw) > 0 {
		if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
			return "", fmt.Errorf("failed to parse identity provider definition: %w", err)
		}
	}
	alias := idpDef.Alias
	if alias == "" {
		alias = idp.Name
	}
	return alias, nil
}

func (r *KeycloakIdentityProviderMapperReconciler) deleteMapper(ctx context.Context, mapper *keycloakv1beta1.KeycloakIdentityProviderMapper) error {
	if mapper.Status.MapperID == "" {
		return nil
	}

	kc, realmName, alias, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		return err
	}

	return kc.DeleteIdentityProviderMapper(ctx, realmName, alias, mapper.Status.MapperID)
}

func (r *KeycloakIdentityProviderMapperReconciler) updateStatus(ctx context.Context, mapper *keycloakv1beta1.KeycloakIdentityProviderMapper, ready bool, status, message, mapperID, mapperName, alias string) (ctrl.Result, error) {
	mapper.Status.Ready = ready
	mapper.Status.Status = status
	mapper.Status.Message = message
	mapper.Status.MapperID = mapperID
	mapper.Status.MapperName = mapperName
	mapper.Status.IdentityProviderAlias = alias

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

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakIdentityProviderMapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakIdentityProviderMapper{}).
		Watches(
			&keycloakv1beta1.KeycloakIdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.findMappersForIdentityProvider),
		).
		Complete(r)
}

// findMappersForIdentityProvider maps a KeycloakIdentityProvider to the
// KeycloakIdentityProviderMappers that reference it, so mappers are requeued
// when their parent IdP transitions to Ready.
func (r *KeycloakIdentityProviderMapperReconciler) findMappersForIdentityProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	idp, ok := obj.(*keycloakv1beta1.KeycloakIdentityProvider)
	if !ok {
		return nil
	}

	var mapperList keycloakv1beta1.KeycloakIdentityProviderMapperList
	if err := r.List(ctx, &mapperList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, m := range mapperList.Items {
		if m.Spec.IdentityProviderRef.Name == idp.Name && m.Namespace == idp.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      m.Name,
					Namespace: m.Namespace,
				},
			})
		}
	}
	return requests
}
