package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakOrganizationReconciler reconciles a KeycloakOrganization object
type KeycloakOrganizationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakorganizations/finalizers,verbs=update

// Reconcile handles KeycloakOrganization reconciliation
func (r *KeycloakOrganizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	org := &keycloakv1beta1.KeycloakOrganization{}
	if err := r.Get(ctx, req.NamespacedName, org); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakOrganization")
		return ctrl.Result{}, err
	}

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

	// Add finalizer
	if !controllerutil.ContainsFinalizer(org, FinalizerName) {
		controllerutil.AddFinalizer(org, FinalizerName)
		if err := r.Update(ctx, org); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, org)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, org, false, "Error", err.Error())
	}

	// Parse organization definition
	var orgDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(org.Spec.Definition.Raw, &orgDef); err != nil {
		log.Error(err, "failed to parse organization definition")
		return r.updateStatus(ctx, org, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Check if organization exists
	if org.Status.OrganizationID == "" {
		// Create
		orgID, err := kc.CreateOrganization(ctx, realmName, org.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create organization")
			return r.updateStatus(ctx, org, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created organization", "name", orgDef.Name, "id", orgID)
		org.Status.OrganizationID = orgID
		return r.updateStatus(ctx, org, true, "Created", "Organization created")
	}

	// Update
	if err := kc.UpdateOrganization(ctx, realmName, org.Status.OrganizationID, org.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update organization")
		return r.updateStatus(ctx, org, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated organization", "name", orgDef.Name)
	return r.updateStatus(ctx, org, true, "Ready", "Organization synchronized")
}

func (r *KeycloakOrganizationReconciler) updateStatus(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization, ready bool, status, message string) (ctrl.Result, error) {
	org.Status.Ready = ready
	org.Status.Status = status
	org.Status.Message = message

	if err := r.Status().Update(ctx, org); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakOrganizationReconciler) getKeycloakClientAndRealm(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization) (*keycloak.Client, string, error) {
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      org.Spec.RealmRef.Name,
		Namespace: org.Namespace,
	}
	if org.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *org.Spec.RealmRef.Namespace
	}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get realm: %w", err)
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: realm.Namespace,
	}
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *realm.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get instance: %w", err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", err
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, nil
}

func (r *KeycloakOrganizationReconciler) deleteOrganization(ctx context.Context, org *keycloakv1beta1.KeycloakOrganization) error {
	if org.Status.OrganizationID == "" {
		return nil
	}

	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, org)
	if err != nil {
		return err
	}

	return kc.DeleteOrganization(ctx, realmName, org.Status.OrganizationID)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakOrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakOrganization{}).
		Complete(r)
}
