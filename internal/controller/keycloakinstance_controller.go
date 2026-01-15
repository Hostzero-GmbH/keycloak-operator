package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

const (
	// FinalizerName is the finalizer used by all controllers
	FinalizerName = "keycloak.hostzero.com/finalizer"

	// PreserveResourceAnnotation is the annotation that prevents deletion of the resource in Keycloak
	// when the CR is deleted. Set to "true" to preserve the resource.
	PreserveResourceAnnotation = "keycloak.hostzero.com/preserve-resource"

	// RequeueDelay is the default requeue delay
	RequeueDelay = 10 * time.Second

	// ErrorRequeueDelay is the requeue delay after an error
	ErrorRequeueDelay = 30 * time.Second

	// MinKeycloakMajorVersion is the minimum supported Keycloak major version
	MinKeycloakMajorVersion = 20

	// MinKeycloakVersionString is the human-readable minimum version
	MinKeycloakVersionString = "20.0.0"
)

// ShouldPreserveResource returns true if the resource should be preserved in Keycloak
// when the CR is deleted. This is determined by the PreserveResourceAnnotation.
func ShouldPreserveResource(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	value, ok := annotations[PreserveResourceAnnotation]
	return ok && value == "true"
}

// KeycloakInstanceReconciler reconciles a KeycloakInstance object
type KeycloakInstanceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles KeycloakInstance reconciliation
func (r *KeycloakInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakInstance"

	// Fetch the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted, remove client from manager
			r.ClientManager.RemoveClient(req.String())
			SetKeycloakConnectionStatus(req.Name, req.Namespace, false)
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakInstance")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, instance.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, FinalizerName) {
			// Cleanup logic
			r.ClientManager.RemoveClient(req.String())

			// Remove finalizer
			controllerutil.RemoveFinalizer(instance, FinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, FinalizerName) {
		controllerutil.AddFinalizer(instance, FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get credentials from secret
	cfg, err := r.getKeycloakConfig(ctx, instance, log)
	if err != nil {
		return r.updateStatus(ctx, instance, false, "", "Error", err.Error())
	}

	// Create/get Keycloak client
	kc := r.ClientManager.GetOrCreateClient(req.String(), cfg)

	// Ping Keycloak to verify connection
	if err := kc.Ping(ctx); err != nil {
		log.Error(err, "failed to connect to Keycloak")
		SetKeycloakConnectionStatus(instance.Name, instance.Namespace, false)
		RecordError(controllerName, "connection_failed")
		return r.updateStatus(ctx, instance, false, "", "ConnectionFailed", fmt.Sprintf("Failed to connect: %v", err))
	}

	// Get server version
	version := ""
	serverInfo, err := kc.GetServerInfo(ctx)
	if err != nil {
		log.Error(err, "failed to get server info")
		RecordError(controllerName, "server_info_failed")
		return r.updateStatus(ctx, instance, false, "", "ServerInfoFailed", fmt.Sprintf("Failed to get server info: %v", err))
	}
	if serverInfo != nil && serverInfo.SystemInfo.Version != "" {
		version = serverInfo.SystemInfo.Version
	}

	// Validate Keycloak version
	if version == "" {
		log.Error(nil, "unable to determine Keycloak version")
		RecordError(controllerName, "version_unknown")
		return r.updateStatus(ctx, instance, false, "", "VersionUnknown", "Unable to determine Keycloak version")
	}

	if err := validateKeycloakVersion(version); err != nil {
		log.Error(err, "unsupported Keycloak version", "version", version, "minimum", MinKeycloakVersionString)
		RecordError(controllerName, "version_unsupported")
		return r.updateStatus(ctx, instance, false, version, "VersionUnsupported", err.Error())
	}

	// Update connection status metric
	SetKeycloakConnectionStatus(instance.Name, instance.Namespace, true)

	log.Info("successfully connected to Keycloak", "baseUrl", instance.Spec.BaseUrl, "version", version)
	return r.updateStatus(ctx, instance, true, version, "Ready", "Connected to Keycloak")
}

func (r *KeycloakInstanceReconciler) getKeycloakConfig(ctx context.Context, instance *keycloakv1beta1.KeycloakInstance, log logr.Logger) (keycloak.Config, error) {
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

	// Check for client credentials
	if instance.Spec.Client != nil {
		cfg.ClientID = instance.Spec.Client.ID
		if instance.Spec.Client.Secret != nil {
			cfg.ClientSecret = *instance.Spec.Client.Secret
		}
	}

	return cfg, nil
}

func (r *KeycloakInstanceReconciler) updateStatus(ctx context.Context, instance *keycloakv1beta1.KeycloakInstance, ready bool, version, status, message string) (ctrl.Result, error) {
	instance.Status.Ready = ready
	instance.Status.Status = status
	instance.Status.Message = message
	if version != "" {
		instance.Status.Version = version
	}

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             status,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	if ready {
		condition.Status = metav1.ConditionTrue
	}

	// Update or add condition
	found := false
	for i, c := range instance.Status.Conditions {
		if c.Type == "Ready" {
			instance.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		instance.Status.Conditions = append(instance.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakInstance{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// validateKeycloakVersion checks if the Keycloak version is supported
func validateKeycloakVersion(version string) error {
	// Parse major version from version string (e.g., "20.0.1", "21.1.2", "24.0.0-SNAPSHOT")
	// Strip any suffix like "-SNAPSHOT", "-RC1", etc.
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

	if majorVersion < MinKeycloakMajorVersion {
		return fmt.Errorf("Keycloak version %s is not supported (minimum: %s)", version, MinKeycloakVersionString)
	}

	return nil
}
