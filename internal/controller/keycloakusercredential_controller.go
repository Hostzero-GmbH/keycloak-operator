package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakUserCredentialReconciler reconciles a KeycloakUserCredential object
type KeycloakUserCredentialReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials/finalizers,verbs=update

// Reconcile handles KeycloakUserCredential reconciliation
func (r *KeycloakUserCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakUserCredential
	cred := &keycloakv1beta1.KeycloakUserCredential{}
	if err := r.Get(ctx, req.NamespacedName, cred); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakUserCredential")
		return ctrl.Result{}, err
	}

	// Handle deletion - nothing to clean up for credentials
	if !cred.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cred, FinalizerName) {
			controllerutil.RemoveFinalizer(cred, FinalizerName)
			if err := r.Update(ctx, cred); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(cred, FinalizerName) {
		controllerutil.AddFinalizer(cred, FinalizerName)
		if err := r.Update(ctx, cred); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and user info
	kc, realmName, userID, err := r.getKeycloakClientAndUser(ctx, cred)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, cred, false, "Error", err.Error())
	}

	// Get password from secret
	password, err := r.getPasswordFromSecret(ctx, cred)
	if err != nil {
		log.Error(err, "failed to get password from secret")
		return r.updateStatus(ctx, cred, false, "Error", err.Error())
	}

	// Set the password
	if err := kc.SetPassword(ctx, realmName, userID, password, cred.Spec.Temporary); err != nil {
		log.Error(err, "failed to set password")
		return r.updateStatus(ctx, cred, false, "Error", fmt.Sprintf("Failed to set password: %v", err))
	}

	log.Info("password set for user", "user", userID)
	now := metav1.Now()
	cred.Status.LastUpdated = &now
	return r.updateStatus(ctx, cred, true, "Ready", "Password set successfully")
}

func (r *KeycloakUserCredentialReconciler) updateStatus(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential, ready bool, status, message string) (ctrl.Result, error) {
	cred.Status.Ready = ready
	cred.Status.Status = status
	cred.Status.Message = message

	if err := r.Status().Update(ctx, cred); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakUserCredentialReconciler) getPasswordFromSecret(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential) (string, error) {
	secret := &corev1.Secret{}
	secretNamespace := cred.Namespace
	if cred.Spec.SecretRef.Namespace != nil {
		secretNamespace = *cred.Spec.SecretRef.Namespace
	}
	secretName := types.NamespacedName{
		Name:      cred.Spec.SecretRef.Name,
		Namespace: secretNamespace,
	}

	if err := r.Get(ctx, secretName, secret); err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	passwordKey := cred.Spec.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", fmt.Errorf("password key %q not found in secret", passwordKey)
	}

	return string(password), nil
}

func (r *KeycloakUserCredentialReconciler) getKeycloakClientAndUser(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential) (*keycloak.Client, string, string, error) {
	// Get the referenced user
	user := &keycloakv1beta1.KeycloakUser{}
	userName := types.NamespacedName{
		Name:      cred.Spec.UserRef.Name,
		Namespace: cred.Namespace,
	}
	if cred.Spec.UserRef.Namespace != nil {
		userName.Namespace = *cred.Spec.UserRef.Namespace
	}
	if err := r.Get(ctx, userName, user); err != nil {
		return nil, "", "", fmt.Errorf("failed to get user: %w", err)
	}

	// Get the realm from the user
	realm := &keycloakv1beta1.KeycloakRealm{}
	realmName := types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: user.Namespace,
	}
	if user.Spec.RealmRef.Namespace != nil {
		realmName.Namespace = *user.Spec.RealmRef.Namespace
	}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", "", fmt.Errorf("failed to get realm: %w", err)
	}

	// Get the instance from the realm
	instance := &keycloakv1beta1.KeycloakInstance{}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: realm.Namespace,
	}
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceName.Namespace = *realm.Spec.InstanceRef.Namespace
	}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", "", fmt.Errorf("failed to get instance: %w", err)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", err
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get user ID
	if user.Status.UserID == "" {
		return nil, "", "", fmt.Errorf("user does not have an ID yet")
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmDef.Realm, user.Status.UserID, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakUserCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakUserCredential{}).
		Complete(r)
}
