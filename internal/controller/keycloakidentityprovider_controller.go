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

// KeycloakIdentityProviderReconciler reconciles a KeycloakIdentityProvider object
type KeycloakIdentityProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakidentityproviders/finalizers,verbs=update

// Reconcile handles KeycloakIdentityProvider reconciliation
func (r *KeycloakIdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	idp := &keycloakv1beta1.KeycloakIdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, idp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakIdentityProvider")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !idp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(idp, FinalizerName) {
			if err := r.deleteIdP(ctx, idp); err != nil {
				log.Error(err, "failed to delete IdP from Keycloak")
			}
			controllerutil.RemoveFinalizer(idp, FinalizerName)
			if err := r.Update(ctx, idp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(idp, FinalizerName) {
		controllerutil.AddFinalizer(idp, FinalizerName)
		if err := r.Update(ctx, idp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, idp)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, idp, false, "Error", err.Error())
	}

	// Parse IdP definition
	var idpDef struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(idp.Spec.Definition.Raw, &idpDef); err != nil {
		log.Error(err, "failed to parse IdP definition")
		return r.updateStatus(ctx, idp, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Check if IdP exists
	_, err = kc.GetIdentityProvider(ctx, realmName, idpDef.Alias)
	if err != nil {
		// Create
		if err := kc.CreateIdentityProvider(ctx, realmName, idp.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to create IdP")
			return r.updateStatus(ctx, idp, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created IdP", "alias", idpDef.Alias)
		idp.Status.Alias = idpDef.Alias
		return r.updateStatus(ctx, idp, true, "Created", "Identity provider created")
	}

	// Update
	if err := kc.UpdateIdentityProvider(ctx, realmName, idpDef.Alias, idp.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update IdP")
		return r.updateStatus(ctx, idp, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated IdP", "alias", idpDef.Alias)
	idp.Status.Alias = idpDef.Alias
	return r.updateStatus(ctx, idp, true, "Ready", "Identity provider synchronized")
}

func (r *KeycloakIdentityProviderReconciler) updateStatus(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider, ready bool, status, message string) (ctrl.Result, error) {
	idp.Status.Ready = ready
	idp.Status.Status = status
	idp.Status.Message = message

	if err := r.Status().Update(ctx, idp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakIdentityProviderReconciler) getKeycloakClientAndRealm(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) (*keycloak.Client, string, error) {
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      idp.Spec.RealmRef.Name,
		Namespace: idp.Namespace,
	}
	if idp.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *idp.Spec.RealmRef.Namespace
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

func (r *KeycloakIdentityProviderReconciler) deleteIdP(ctx context.Context, idp *keycloakv1beta1.KeycloakIdentityProvider) error {
	if idp.Status.Alias == "" {
		return nil
	}

	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, idp)
	if err != nil {
		return err
	}

	return kc.DeleteIdentityProvider(ctx, realmName, idp.Status.Alias)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakIdentityProvider{}).
		Complete(r)
}
