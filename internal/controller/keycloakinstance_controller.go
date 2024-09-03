package controller

import (
	"context"
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

// KeycloakInstanceReconciler reconciles a KeycloakInstance object
type KeycloakInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles KeycloakInstance reconciliation
func (r *KeycloakInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakInstance")
		return ctrl.Result{}, err
	}

	// Get credentials from secret
	cfg, err := r.getKeycloakConfig(ctx, instance)
	if err != nil {
		log.Error(err, "failed to get keycloak config")
		return ctrl.Result{}, err
	}

	// Create Keycloak client and verify connection
	kc := keycloak.NewClient(cfg, log)
	if err := kc.Ping(ctx); err != nil {
		log.Error(err, "failed to connect to Keycloak")
		return ctrl.Result{}, err
	}

	log.Info("successfully connected to Keycloak", "baseUrl", instance.Spec.BaseUrl)
	return ctrl.Result{}, nil
}

func (r *KeycloakInstanceReconciler) getKeycloakConfig(ctx context.Context, instance *keycloakv1beta1.KeycloakInstance) (keycloak.Config, error) {
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

	if err := r.Get(ctx, secretName, secret); err != nil {
		return cfg, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	// Extract credentials
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

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakInstance{}).
		Complete(r)
}
