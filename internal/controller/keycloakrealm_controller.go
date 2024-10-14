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
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakRealmReconciler reconciles a KeycloakRealm object
type KeycloakRealmReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances,verbs=get;list;watch

// Reconcile handles KeycloakRealm reconciliation
func (r *KeycloakRealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, req.NamespacedName, realm); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRealm")
		return ctrl.Result{}, err
	}

	// Get Keycloak client for this realm's instance
	kc, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		log.Error(err, "failed to get keycloak client")
		return r.updateStatus(ctx, realm, false, "InstanceNotReady", err.Error())
	}

	// Parse realm definition to get realm name
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		log.Error(err, "failed to parse realm definition")
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", err.Error())
	}

	if realmDef.Realm == "" {
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", "realm name is required in definition")
	}

	// Check if realm exists
	_, err = kc.GetRealm(ctx, realmDef.Realm)
	if err != nil {
		// Realm doesn't exist, create it
		log.Info("creating realm", "realm", realmDef.Realm)
		if err := kc.CreateRealmFromDefinition(ctx, realm.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to create realm")
			return r.updateStatus(ctx, realm, false, "CreateFailed", err.Error())
		}
	} else {
		// Realm exists, update it
		log.Info("updating realm", "realm", realmDef.Realm)
		if err := kc.UpdateRealm(ctx, realmDef.Realm, realm.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to update realm")
			return r.updateStatus(ctx, realm, false, "UpdateFailed", err.Error())
		}
	}

	log.Info("realm reconciled", "realm", realmDef.Realm)
	return r.updateStatus(ctx, realm, true, "Ready", "Realm synchronized")
}

func (r *KeycloakRealmReconciler) updateStatus(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm, ready bool, status, message string) (ctrl.Result, error) {
	realm.Status.Ready = ready
	realm.Status.Status = status
	realm.Status.Message = message

	if err := r.Status().Update(ctx, realm); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeycloakRealmReconciler) getKeycloakClient(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm) (*keycloak.Client, error) {
	// Get the instance reference
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	// Get the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	// Check if instance is ready
	if !instance.Status.Ready {
		return nil, fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	// Build config from instance
	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRealm{}).
		Complete(r)
}

// GetKeycloakConfigFromInstance builds a keycloak config from an instance
func GetKeycloakConfigFromInstance(ctx context.Context, c client.Client, instance *keycloakv1beta1.KeycloakInstance) (keycloak.Config, error) {
	cfg := keycloak.Config{
		BaseURL: instance.Spec.BaseUrl,
	}

	if instance.Spec.Realm != nil {
		cfg.Realm = *instance.Spec.Realm
	}

	// Get credentials secret
	secret := &corev1.Secret{}
	secretNamespace := instance.Namespace
	if instance.Spec.Credentials.SecretRef.Namespace != nil {
		secretNamespace = *instance.Spec.Credentials.SecretRef.Namespace
	}
	secretName := types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretRef.Name,
		Namespace: secretNamespace,
	}

	if err := c.Get(ctx, secretName, secret); err != nil {
		return cfg, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	usernameKey := instance.Spec.Credentials.SecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := instance.Spec.Credentials.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	if username, ok := secret.Data[usernameKey]; ok {
		cfg.Username = string(username)
	} else {
		return cfg, fmt.Errorf("username key %q not found in secret", usernameKey)
	}

	if password, ok := secret.Data[passwordKey]; ok {
		cfg.Password = string(password)
	} else {
		return cfg, fmt.Errorf("password key %q not found in secret", passwordKey)
	}

	return cfg, nil
}
