package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

// KeycloakRealmReconciler reconciles a KeycloakRealm object
type KeycloakRealmReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles KeycloakRealm reconciliation
func (r *KeycloakRealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakRealm"

	// Fetch the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, req.NamespacedName, realm); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakRealm")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, realm.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !realm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(realm, FinalizerName) {
			// Delete realm from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(realm) {
				log.Info("preserving realm in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteRealm(ctx, realm); err != nil {
				log.Error(err, "failed to delete realm from Keycloak")
				// Continue with finalizer removal even on error
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(realm, FinalizerName)
			if err := r.Update(ctx, realm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(realm, FinalizerName) {
		controllerutil.AddFinalizer(realm, FinalizerName)
		if err := r.Update(ctx, realm); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client for this realm's instance
	kc, instanceRef, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		RecordError(controllerName, "instance_not_ready")
		return r.updateStatus(ctx, realm, false, "InstanceNotReady", err.Error(), instanceRef)
	}

	// Parse realm definition to extract realm name
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", fmt.Sprintf("Failed to parse realm definition: %v", err), instanceRef)
	}

	// Ensure realm name is set
	if realmDef.Realm == "" {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, realm, false, "InvalidDefinition", "Realm name is required in definition", instanceRef)
	}

	// Build the effective definition, injecting SMTP credentials from secret if configured
	definition := realm.Spec.Definition.Raw
	if realm.Spec.SmtpSecretRef != nil {
		smtpUser, smtpPassword, err := r.resolveSmtpSecret(ctx, realm)
		if err != nil {
			RecordError(controllerName, "secret_error")
			return r.updateStatus(ctx, realm, false, "SmtpSecretError", err.Error(), instanceRef)
		}
		definition = mergeSmtpCredentials(definition, smtpUser, smtpPassword)
	}

	// Check if realm exists
	existingRealm, err := kc.GetRealm(ctx, realmDef.Realm)
	if err != nil {
		// Realm doesn't exist, create it
		log.Info("creating realm", "realm", realmDef.Realm)
		if err := kc.CreateRealmFromDefinition(ctx, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, realm, false, "CreateFailed", fmt.Sprintf("Failed to create realm: %v", err), instanceRef)
		}
		log.Info("realm created successfully", "realm", realmDef.Realm)
	} else {
		// Realm exists, update it - merge ID into definition
		log.Info("updating realm", "realm", realmDef.Realm)
		definition = mergeIDIntoDefinition(definition, existingRealm.ID)
		if err := kc.UpdateRealm(ctx, realmDef.Realm, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, realm, false, "UpdateFailed", fmt.Sprintf("Failed to update realm: %v", err), instanceRef)
		}
		log.Info("realm updated successfully", "realm", realmDef.Realm)
	}

	// Update status
	realm.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s", realmDef.Realm)
	return r.updateStatus(ctx, realm, true, "Ready", "Realm synchronized", instanceRef)
}

func (r *KeycloakRealmReconciler) getKeycloakClient(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm) (*keycloak.Client, *keycloakv1beta1.InstanceRef, error) {
	// Get the instance reference
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}

	// Create instance ref for status
	instanceRef := &keycloakv1beta1.InstanceRef{
		InstanceRef: fmt.Sprintf("%s/%s", instanceNamespace, realm.Spec.InstanceRef.Name),
	}

	// Get the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, instanceRef, fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	// Check if instance is ready
	if !instance.Status.Ready {
		return nil, instanceRef, fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	// Build config from instance
	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, instanceRef, fmt.Errorf("failed to get Keycloak config: %w", err)
	}

	// Get the Keycloak client from manager
	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, instanceRef, fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, instanceRef, nil
}

func (r *KeycloakRealmReconciler) deleteRealm(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm) error {
	kc, _, err := r.getKeycloakClient(ctx, realm)
	if err != nil {
		return err
	}

	// Parse realm definition to get realm name
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return fmt.Errorf("failed to parse realm definition: %w", err)
	}

	if realmDef.Realm == "" {
		return fmt.Errorf("realm name not found in definition")
	}

	return kc.DeleteRealm(ctx, realmDef.Realm)
}

func (r *KeycloakRealmReconciler) updateStatus(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm, ready bool, status, message string, instanceRef *keycloakv1beta1.InstanceRef) (ctrl.Result, error) {
	realm.Status.Ready = ready
	realm.Status.Status = status
	realm.Status.Message = message
	realm.Status.Instance = instanceRef

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
	for i, c := range realm.Status.Conditions {
		if c.Type == "Ready" {
			realm.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		realm.Status.Conditions = append(realm.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, realm); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

func (r *KeycloakRealmReconciler) resolveSmtpSecret(ctx context.Context, realm *keycloakv1beta1.KeycloakRealm) (string, string, error) {
	ref := realm.Spec.SmtpSecretRef
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: realm.Namespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get SMTP secret %q: %w", ref.Name, err)
	}

	userKey := ref.UserKey
	if userKey == "" {
		userKey = "user"
	}
	passwordKey := ref.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	user, ok := secret.Data[userKey]
	if !ok {
		return "", "", fmt.Errorf("key %q not found in SMTP secret %q", userKey, ref.Name)
	}
	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("key %q not found in SMTP secret %q", passwordKey, ref.Name)
	}

	return string(user), string(password), nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakRealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakRealm{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findRealmsForSecret),
		).
		Complete(r)
}

// findRealmsForSecret maps a Secret to the KeycloakRealms that reference it via smtpSecretRef
func (r *KeycloakRealmReconciler) findRealmsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)

	var realmList keycloakv1beta1.KeycloakRealmList
	if err := r.List(ctx, &realmList, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, realm := range realmList.Items {
		if realm.Spec.SmtpSecretRef != nil && realm.Spec.SmtpSecretRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      realm.Name,
					Namespace: realm.Namespace,
				},
			})
		}
	}
	return requests
}
