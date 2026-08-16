package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

// KeycloakIdentityProviderReconciler reconciles a KeycloakIdentityProvider object
type KeycloakIdentityProviderReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations,verbs=get;list;watch

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
			// Delete identity provider from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(idp) {
				log.Info("preserving identity provider in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else {
				// Best-effort cleanup of the operator-managed token-exchange policy
				// in realm-management's authz resource server before the IdP itself
				// goes away. Errors are logged but don't block deletion.
				if kc, realmName, resolveErr := r.getKeycloakClientAndRealm(ctx, idp); resolveErr == nil {
					if cleanupErr := r.cleanupTokenExchange(ctx, kc, realmName, idp); cleanupErr != nil {
						log.Error(cleanupErr, "failed to clean up token-exchange policy")
					}
				}
				if err := r.deleteIdentityProvider(ctx, idp); err != nil {
					log.Error(err, "failed to delete identity provider from Keycloak")
				}
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

	// Parse identity provider definition to extract alias and reject inline organizationId
	var idpDef struct {
		Alias          string `json:"alias"`
		OrganizationID string `json:"organizationId"`
	}
	if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", fmt.Sprintf("Failed to parse identity provider definition: %v", err), "")
	}
	if idpDef.OrganizationID != "" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", "definition.organizationId is not supported; use spec.organizationRef", "")
	}

	// Resolve the alias from spec.alias.
	alias, err := resolveIdentifier("alias", idp.Spec.Alias, idpDef.Alias)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, idp, false, InvalidIdentifierReason, err.Error(), "")
	}
	idp.Status.Alias = alias

	// Prepare definition with alias set
	definition := setFieldInDefinition(idp.Spec.Definition.Raw, "alias", alias)

	// Merge config values from secret if configured
	if idp.Spec.ConfigSecretRef != nil {
		secretData, err := r.resolveConfigSecret(ctx, idp)
		if err != nil {
			RecordError(controllerName, "secret_error")
			return r.updateStatus(ctx, idp, false, "ConfigSecretError", err.Error(), alias)
		}
		definition = mergeIDPConfig(definition, secretData)
	}

	orgID, err := r.resolveOrganization(ctx, idp)
	if err != nil {
		if isOrganizationRealmMismatch(err) {
			RecordError(controllerName, "organization_realm_mismatch")
			return r.updateStatus(ctx, idp, false, "OrganizationRealmMismatch", err.Error(), alias)
		}
		RecordError(controllerName, "organization_not_ready")
		return r.updateStatus(ctx, idp, false, "OrganizationNotReady", err.Error(), alias)
	}
	idp.Status.OrganizationID = orgID
	if orgID != "" {
		definition = setFieldInDefinition(definition, "organizationId", orgID)
	}

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
		// Identity provider exists — check if update is needed (drift-detection, pace patch)
		// to avoid reconcile-storms where every 5-min sync triggers an unneeded PUT.
		currentRaw, fetchErr := kc.GetIdentityProviderRaw(ctx, realmName, alias)
		needsUpdate := true
		if fetchErr != nil {
			log.Error(fetchErr, "failed to fetch current IdP state, falling through to update")
		} else if currentRaw != nil {
			needsUpdate = !idpDefinitionsMatch(definition, currentRaw)
		}

		if needsUpdate {
			log.Info("updating identity provider", "alias", alias, "realm", realmName)
			if err := kc.UpdateIdentityProvider(ctx, realmName, alias, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, idp, false, "UpdateFailed", fmt.Sprintf("Failed to update identity provider: %v", err), alias)
			}
			log.Info("identity provider updated successfully", "alias", alias)
		} else {
			log.V(1).Info("identity provider already in sync, skipping update", "alias", alias)
		}
	}

	// Reconcile the token-exchange permission, if managed. Failure here is
	// surfaced on status.tokenExchange.message but does not flip the parent
	// Ready=false — the IdP itself is in sync regardless of the TE side, and
	// flapping the parent Ready bit on transient authz-API hiccups would
	// cascade into dependent resources (KeycloakIdentityProviderMapper, etc.).
	if idp.Spec.TokenExchange != nil {
		teStatus, teErr := r.reconcileTokenExchange(ctx, kc, realmName, alias, idp)
		switch {
		case teErr == nil:
			idp.Status.TokenExchange = teStatus
		case IsTokenExchangeWaiting(teErr):
			// Soft wait — referenced state (typically one of the allowedClients)
			// isn't there yet. Log at INFO level and surface a friendly status
			// message; the requeue happens via updateStatus's RequeueAfter.
			log.Info("token-exchange reconcile waiting", "alias", alias, "reason", teErr.Error())
			if idp.Status.TokenExchange == nil {
				idp.Status.TokenExchange = &keycloakv1beta1.IDPTokenExchangeStatus{}
			}
			idp.Status.TokenExchange.Message = teErr.Error()
		default:
			RecordError(controllerName, "tokenexchange_error")
			log.Error(teErr, "failed to reconcile token-exchange permission", "alias", alias)
			if idp.Status.TokenExchange == nil {
				idp.Status.TokenExchange = &keycloakv1beta1.IDPTokenExchangeStatus{}
			}
			idp.Status.TokenExchange.Message = teErr.Error()
		}
	}

	// Update status
	idp.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s", realmName, alias)
	return r.updateStatus(ctx, idp, true, "Ready", "Identity provider synchronized", alias)
}

func (r *KeycloakIdentityProviderReconciler) getKeycloakClientAndRealm(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) (*keycloak.Client, string, error) {
	return GetKeycloakClientAndRealmForIDP(ctx, r.Client, r.ClientManager, idp)
}

type organizationRealmMismatchError struct {
	org types.NamespacedName
	idp types.NamespacedName
}

func (e organizationRealmMismatchError) Error() string {
	return fmt.Sprintf("KeycloakOrganization %s is not in the same realm as identity provider %s", e.org, e.idp)
}

func isOrganizationRealmMismatch(err error) bool {
	_, ok := err.(organizationRealmMismatchError)
	return ok
}

// resolveOrganization returns the Keycloak organization ID for spec.organizationRef.
// An empty string means the IdP is not linked to an organization.
func (r *KeycloakIdentityProviderReconciler) resolveOrganization(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) (string, error) {
	if idp.Spec.OrganizationRef == nil {
		return "", nil
	}

	orgKey := types.NamespacedName{
		Name:      idp.Spec.OrganizationRef.Name,
		Namespace: idp.Namespace,
	}
	org := &keycloakv1beta1.KeycloakOrganization{}
	if err := r.Get(ctx, orgKey, org); err != nil {
		return "", fmt.Errorf("failed to get KeycloakOrganization %s: %w", orgKey, err)
	}
	if !org.Status.Ready || org.Status.OrganizationID == "" {
		return "", fmt.Errorf("KeycloakOrganization %s is not ready", orgKey)
	}
	if !sameRealmPlacement(idp, org) {
		return "", organizationRealmMismatchError{org: orgKey, idp: types.NamespacedName{Name: idp.Name, Namespace: idp.Namespace}}
	}
	return org.Status.OrganizationID, nil
}

// sameRealmPlacement reports whether the IdP and organization reference the same realm CR.
func sameRealmPlacement(idp *keycloakv1beta1.KeycloakIdentityProvider, org *keycloakv1beta1.KeycloakOrganization) bool {
	if idp.Spec.RealmRef != nil && org.Spec.RealmRef != nil {
		return idp.Spec.RealmRef.Name == org.Spec.RealmRef.Name
	}
	if idp.Spec.ClusterRealmRef != nil && org.Spec.ClusterRealmRef != nil {
		return idp.Spec.ClusterRealmRef.Name == org.Spec.ClusterRealmRef.Name
	}
	return false
}

func (r *KeycloakIdentityProviderReconciler) deleteIdentityProvider(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, idp)
	if err != nil {
		return err
	}

	// Use spec.alias so deletion targets the synchronized identity provider.
	// Empty means never synchronized (unmigrated object).
	alias := identifierValue(idp.Spec.Alias)
	if alias == "" {
		return nil
	}
	return kc.DeleteIdentityProvider(ctx, realmName, alias)
}

func (r *KeycloakIdentityProviderReconciler) updateStatus(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider, ready bool, status, message, alias string) (ctrl.Result, error) {
	idp.Status.Ready = ready
	idp.Status.Status = status
	idp.Status.Message = message

	idp.Status.Conditions = setReadyCondition(idp.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, idp, ready)
}

// resolveConfigSecret reads all keys from a referenced Secret (upstream ConfigSecretRef path).
func (r *KeycloakIdentityProviderReconciler) resolveConfigSecret(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) (map[string]string, error) {
	ref := idp.Spec.ConfigSecretRef
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: idp.Namespace}, secret); err != nil {
		return nil, fmt.Errorf("failed to get config secret %q: %w", ref.Name, err)
	}

	data := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		data[k] = string(v)
	}
	return data, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakIdentityProvider{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findIDPsForSecret),
		).
		Watches(
			&keycloakv1beta1.KeycloakOrganization{},
			handler.EnqueueRequestsFromMapFunc(r.findIDPsForOrganization),
		).
		Complete(r)
}

// findIDPsForSecret maps a Secret to the KeycloakIdentityProviders that reference it via configSecretRef
func (r *KeycloakIdentityProviderReconciler) findIDPsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)

	var idpList keycloakv1beta1.KeycloakIdentityProviderList
	if err := r.List(ctx, &idpList, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, idp := range idpList.Items {
		if idp.Spec.ConfigSecretRef != nil && idp.Spec.ConfigSecretRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      idp.Name,
					Namespace: idp.Namespace,
				},
			})
		}
	}
	return requests
}

// findIDPsForOrganization maps a KeycloakOrganization to the identity providers
// that reference it via organizationRef.
func (r *KeycloakIdentityProviderReconciler) findIDPsForOrganization(ctx context.Context, obj client.Object) []reconcile.Request {
	org := obj.(*keycloakv1beta1.KeycloakOrganization)

	var idpList keycloakv1beta1.KeycloakIdentityProviderList
	if err := r.List(ctx, &idpList, client.InNamespace(org.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, idp := range idpList.Items {
		if idp.Spec.OrganizationRef != nil && idp.Spec.OrganizationRef.Name == org.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      idp.Name,
					Namespace: idp.Namespace,
				},
			})
		}
	}
	return requests
}
