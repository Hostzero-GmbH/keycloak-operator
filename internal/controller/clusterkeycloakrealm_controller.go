package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

// ClusterKeycloakRealmReconciler reconciles a ClusterKeycloakRealm object
type ClusterKeycloakRealmReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=clusterkeycloakrealms/finalizers,verbs=update

// Reconcile handles ClusterKeycloakRealm reconciliation
func (r *ClusterKeycloakRealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	realm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, req.NamespacedName, realm); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch ClusterKeycloakRealm")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !realm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(realm, FinalizerName) {
			if err := r.deleteRealm(ctx, realm); err != nil {
				log.Error(err, "failed to delete realm from Keycloak")
			}
			controllerutil.RemoveFinalizer(realm, FinalizerName)
			if err := r.Update(ctx, realm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(realm, FinalizerName) {
		controllerutil.AddFinalizer(realm, FinalizerName)
		if err := r.Update(ctx, realm); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client
	kc, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, realm, false, "Error", err.Error())
	}

	// Parse realm definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		log.Error(err, "failed to parse realm definition")
		return r.updateStatus(ctx, realm, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Check if realm exists
	_, err = kc.GetRealm(ctx, realmDef.Realm)
	if err != nil {
		// Create realm
		if err := kc.CreateRealmFromDefinition(ctx, realm.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to create realm")
			return r.updateStatus(ctx, realm, false, "Error", fmt.Sprintf("Failed to create: %v", err))
		}
		log.Info("created realm", "realm", realmDef.Realm)
		realm.Status.ResourcePath = "/admin/realms/" + realmDef.Realm
		return r.updateStatus(ctx, realm, true, "Created", "Realm created successfully")
	}

	// Update realm
	if err := kc.UpdateRealm(ctx, realmDef.Realm, realm.Spec.Definition.Raw); err != nil {
		log.Error(err, "failed to update realm")
		return r.updateStatus(ctx, realm, false, "Error", fmt.Sprintf("Failed to update: %v", err))
	}

	log.Info("updated realm", "realm", realmDef.Realm)
	realm.Status.ResourcePath = "/admin/realms/" + realmDef.Realm
	return r.updateStatus(ctx, realm, true, "Ready", "Realm synchronized")
}

func (r *ClusterKeycloakRealmReconciler) updateStatus(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm, ready bool, status, message string) (ctrl.Result, error) {
	realm.Status.Ready = ready
	realm.Status.Status = status
	realm.Status.Message = message

	if err := r.Status().Update(ctx, realm); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterKeycloakRealmReconciler) getKeycloakClient(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm) (*keycloak.Client, error) {
	// Get cluster instance
	instance := &keycloakv1beta1.ClusterKeycloakInstance{}
	if err := r.Get(ctx, types.NamespacedName{Name: realm.Spec.ClusterInstanceRef.Name}, instance); err != nil {
		return nil, fmt.Errorf("failed to get cluster instance: %w", err)
	}

	cfg := keycloak.Config{
		BaseURL: instance.Spec.BaseUrl,
	}
	if instance.Spec.Realm != nil {
		cfg.Realm = *instance.Spec.Realm
	}

	// Get credentials
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretRef.Name,
		Namespace: instance.Spec.Credentials.SecretRef.Namespace,
	}
	if err := r.Get(ctx, secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	usernameKey := instance.Spec.Credentials.SecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := instance.Spec.Credentials.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	cfg.Username = string(secret.Data[usernameKey])
	cfg.Password = string(secret.Data[passwordKey])

	return keycloak.NewClient(cfg, log.FromContext(ctx)), nil
}

func (r *ClusterKeycloakRealmReconciler) deleteRealm(ctx context.Context, realm *keycloakv1beta1.ClusterKeycloakRealm) error {
	kc, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		return err
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return err
	}

	return kc.DeleteRealm(ctx, realmDef.Realm)
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterKeycloakRealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.ClusterKeycloakRealm{}).
		Complete(r)
}
