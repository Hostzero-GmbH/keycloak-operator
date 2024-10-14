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
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakUserReconciler reconciles a KeycloakUser object
type KeycloakUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers/finalizers,verbs=update

// Reconcile handles KeycloakUser reconciliation
func (r *KeycloakUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakUser
	user := &keycloakv1beta1.KeycloakUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakUser")
		return ctrl.Result{}, err
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, user)
	if err != nil {
		log.Error(err, "failed to get keycloak client")
		return r.updateStatus(ctx, user, false, "RealmNotReady", err.Error())
	}

	// Parse user definition
	var userDef struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(user.Spec.Definition.Raw, &userDef); err != nil {
		log.Error(err, "failed to parse user definition")
		return r.updateStatus(ctx, user, false, "InvalidDefinition", err.Error())
	}

	if userDef.Username == "" {
		return r.updateStatus(ctx, user, false, "InvalidDefinition", "username is required in definition")
	}

	// Check if user exists
	existingUser, err := kc.GetUserByUsername(ctx, realmName, userDef.Username)
	if err != nil {
		// User doesn't exist, create it
		log.Info("creating user", "username", userDef.Username)
		_, err = kc.CreateUser(ctx, realmName, user.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create user")
			return r.updateStatus(ctx, user, false, "CreateFailed", err.Error())
		}
	} else {
		// User exists, update it
		log.Info("updating user", "username", userDef.Username)
		if err := kc.UpdateUser(ctx, realmName, *existingUser.ID, user.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to update user")
			return r.updateStatus(ctx, user, false, "UpdateFailed", err.Error())
		}
	}

	log.Info("user reconciled", "username", userDef.Username)
	return r.updateStatus(ctx, user, true, "Ready", "User synchronized")
}

func (r *KeycloakUserReconciler) updateStatus(ctx context.Context, user *keycloakv1beta1.KeycloakUser, ready bool, status, message string) (ctrl.Result, error) {
	user.Status.Ready = ready
	user.Status.Status = status
	user.Status.Message = message

	if err := r.Status().Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeycloakUserReconciler) getKeycloakClientAndRealm(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (*keycloak.Client, string, error) {
	// Get the realm
	realmNamespace := user.Namespace
	if user.Spec.RealmRef.Namespace != nil {
		realmNamespace = *user.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get instance
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", err
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakUser{}).
		Complete(r)
}
