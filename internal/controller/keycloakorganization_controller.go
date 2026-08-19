package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
			// Delete organization from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(org) {
				log.Info("preserving organization in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteOrganization(ctx, org); err != nil {
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

	// Resolve the organization name from spec.name. NOTE: organizations use a
	// typed-struct round-trip, so any definition field not present on
	// keycloak.OrganizationRepresentation is dropped before send. This is a known
	// passthrough limitation.
	orgName, err := resolveIdentifier("name", org.Spec.Name, orgDef.Name)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, org, false, InvalidIdentifierReason, err.Error(), "")
	}
	orgDef.Name = orgName
	org.Status.OrganizationName = orgName

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
		// Skip the PUT when the server already matches the spec.
		orgID = existingOrg.ID
		orgDef.ID = orgID

		currentRaw, fetchErr := kc.GetOrganizationRaw(ctx, realmName, orgID)
		needsUpdate := true
		if fetchErr != nil {
			log.Error(fetchErr, "failed to fetch current organization state, falling through to update")
		} else if currentRaw != nil {
			needsUpdate = !organizationDefinitionsMatch(org.Spec.Definition.Raw, currentRaw)
		}

		if needsUpdate {
			log.Info("updating organization", "name", orgDef.Name, "realm", realmName)
			if err := kc.UpdateOrganization(ctx, realmName, orgDef); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, org, false, "UpdateFailed", fmt.Sprintf("Failed to update organization: %v", err), orgID)
			}
			log.Info("organization updated successfully", "name", orgDef.Name)
		} else {
			log.V(1).Info("organization already in sync, skipping update", "name", orgDef.Name)
		}
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
	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, org.Namespace, org.Spec.RealmRef, org.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", "", err
	}
	return res.Client, res.RealmName, res.Version, nil
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

	org.Status.Conditions = setReadyCondition(org.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, org, ready)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakOrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakOrganization{}).
		Complete(r)
}
