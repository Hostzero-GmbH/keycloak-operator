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
	"github.com/Hostzero-GmbH/keycloak-operator/internal/telemetry"
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
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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

	// Parse identity provider definition to extract alias and reject inline
	// organization links: organizationId (Keycloak < 26.8) and
	// organizationLinks (26.8+) both carry realm-specific organization UUIDs.
	var idpDef struct {
		Alias             string          `json:"alias"`
		OrganizationID    string          `json:"organizationId"`
		OrganizationLinks json.RawMessage `json:"organizationLinks"`
	}
	if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", fmt.Sprintf("Failed to parse identity provider definition: %v", err), "")
	}
	if idpDef.OrganizationID != "" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", "definition.organizationId is not supported; use spec.organizationRef", "")
	}
	if len(idpDef.OrganizationLinks) > 0 && string(idpDef.OrganizationLinks) != "null" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, idp, false, "InvalidDefinition", "definition.organizationLinks is not supported; use spec.organizationRef", "")
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

	definition, err = applyConfigSecret(ctx, r.Client, idp.Namespace, idp.Spec.ConfigSecretRef, definition, false)
	if err != nil {
		RecordError(controllerName, "secret_error")
		return r.updateStatus(ctx, idp, false, "ConfigSecretError", err.Error(), alias)
	}

	// The organization link is not part of the identity provider
	// representation: Keycloak 26.8 moved it to a many-to-many table and
	// ignores organizationId on POST/PUT, so it is reconciled through the
	// organization endpoints after the identity provider exists (#198).
	// status.organizationID records the organization the IdP is currently
	// linked to, which is what a later reconcile unlinks from when the
	// reference changes or goes away.
	previousOrgID := idp.Status.OrganizationID
	orgID, err := r.resolveOrganization(ctx, idp)
	if err != nil {
		if isOrganizationRealmMismatch(err) {
			RecordError(controllerName, "organization_realm_mismatch")
			return r.updateStatus(ctx, idp, false, "OrganizationRealmMismatch", err.Error(), alias)
		}
		RecordError(controllerName, "organization_not_ready")
		return r.updateStatus(ctx, idp, false, "OrganizationNotReady", err.Error(), alias)
	}

	desiredHash := definitionHash(definition)

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
		idp.Status.LastAppliedDefinitionHash = desiredHash
		log.Info("identity provider created successfully", "alias", alias)
	} else {
		// Identity provider exists — check if update is needed (drift-detection)
		// to avoid reconcile-storms where every 5-min sync triggers an unneeded
		// PUT. idpDefinitionsMatch cannot see changes to the masked
		// clientSecret, so a hash mismatch against the last applied definition
		// forces the PUT (#143).
		needsUpdate := desiredHash != idp.Status.LastAppliedDefinitionHash
		if !needsUpdate {
			currentRaw, fetchErr := kc.GetIdentityProviderRaw(ctx, realmName, alias)
			if fetchErr != nil {
				log.Error(fetchErr, "failed to fetch current IdP state, falling through to update")
				needsUpdate = true
			} else {
				needsUpdate = currentRaw == nil || !idpDefinitionsMatch(definition, currentRaw)
			}
		}

		if needsUpdate {
			log.Info("updating identity provider", "alias", alias, "realm", realmName)
			if err := kc.UpdateIdentityProvider(ctx, realmName, alias, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, idp, false, "UpdateFailed", fmt.Sprintf("Failed to update identity provider: %v", err), alias)
			}
			idp.Status.LastAppliedDefinitionHash = desiredHash
			log.Info("identity provider updated successfully", "alias", alias)
		} else {
			log.V(1).Info("identity provider already in sync, skipping update", "alias", alias)
		}
	}

	// Reconcile the organization link now that the IdP exists. On Keycloak
	// 26.0 a PUT without organizationId drops the link, so this has to run
	// after the create/update above; on 26.8 the representation never
	// carries the link in the first place.
	if err := r.reconcileOrganizationLink(ctx, kc, realmName, alias, previousOrgID, orgID); err != nil {
		RecordError(controllerName, "keycloak_api_error")
		return r.updateStatus(ctx, idp, false, "OrganizationLinkFailed", fmt.Sprintf("Failed to reconcile organization link: %v", err), alias)
	}
	idp.Status.OrganizationID = orgID

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

// reconcileOrganizationLink makes the organization side of Keycloak match
// desiredOrgID for the identity provider alias: the IdP is unlinked from
// previousOrgID when that differs from the desired organization, and linked to
// desiredOrgID unless it already is. Both steps consult
// GET /organizations/{org}/identity-providers first, so a steady-state
// reconcile issues no writes. Links to organizations the operator does not
// know about are left alone.
func (r *KeycloakIdentityProviderReconciler) reconcileOrganizationLink(ctx context.Context, kc *keycloak.Client, realmName, alias, previousOrgID, desiredOrgID string) error {
	log := log.FromContext(ctx)

	if previousOrgID != "" && previousOrgID != desiredOrgID {
		linked, err := organizationHasIdentityProvider(ctx, kc, realmName, previousOrgID, alias)
		if err != nil {
			return fmt.Errorf("failed to list identity providers of organization %s: %w", previousOrgID, err)
		}
		if linked {
			log.Info("unlinking identity provider from organization", "alias", alias, "organizationID", previousOrgID)
			if err := kc.UnlinkIdentityProviderFromOrganization(ctx, realmName, previousOrgID, alias); err != nil {
				return fmt.Errorf("failed to unlink identity provider from organization %s: %w", previousOrgID, err)
			}
		}
	}

	if desiredOrgID == "" {
		return nil
	}

	linked, err := organizationHasIdentityProvider(ctx, kc, realmName, desiredOrgID, alias)
	if err != nil {
		return fmt.Errorf("failed to list identity providers of organization %s: %w", desiredOrgID, err)
	}
	if linked {
		log.V(1).Info("identity provider already linked to organization", "alias", alias, "organizationID", desiredOrgID)
		return nil
	}
	log.Info("linking identity provider to organization", "alias", alias, "organizationID", desiredOrgID)
	if err := kc.LinkIdentityProviderToOrganization(ctx, realmName, desiredOrgID, alias); err != nil {
		return fmt.Errorf("failed to link identity provider to organization %s: %w", desiredOrgID, err)
	}
	return nil
}

// organizationHasIdentityProvider reports whether the organization lists the
// identity provider alias among its linked providers. An organization that no
// longer exists (404) has no links, which lets a stale status.organizationID
// be cleared without an error.
func organizationHasIdentityProvider(ctx context.Context, kc *keycloak.Client, realmName, orgID, alias string) (bool, error) {
	idps, err := kc.GetOrganizationIdentityProviders(ctx, realmName, orgID)
	if err != nil {
		if keycloak.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, idp := range idps {
		if idp.Alias != nil && *idp.Alias == alias {
			return true, nil
		}
	}
	return false, nil
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

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakIdentityProvider{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findIDPsForSecret),
		).
		Watches(
			&keycloakv1beta1.KeycloakOrganization{},
			handler.EnqueueRequestsFromMapFunc(r.findIDPsForOrganization),
		)
	return watchRealms(b, r.Client,
		func() client.ObjectList { return &keycloakv1beta1.KeycloakIdentityProviderList{} },
		func(o client.Object) (*keycloakv1beta1.ResourceRef, *keycloakv1beta1.ClusterResourceRef) {
			idp := o.(*keycloakv1beta1.KeycloakIdentityProvider)
			return idp.Spec.RealmRef, idp.Spec.ClusterRealmRef
		},
	).Complete(telemetry.WrapReconciler("KeycloakIdentityProvider", r))
}

// findIDPsForSecret maps a Secret to the KeycloakIdentityProviders that reference it via configSecretRef
func (r *KeycloakIdentityProviderReconciler) findIDPsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	return findForConfigSecret(ctx, r.Client, obj.(*corev1.Secret), &keycloakv1beta1.KeycloakIdentityProviderList{}, func(o client.Object) *keycloakv1beta1.ConfigSecretRef {
		return o.(*keycloakv1beta1.KeycloakIdentityProvider).Spec.ConfigSecretRef
	})
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
