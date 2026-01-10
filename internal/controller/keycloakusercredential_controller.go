package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

// KeycloakUserCredentialReconciler reconciles a KeycloakUserCredential object
type KeycloakUserCredentialReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusercredentials/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakusers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles KeycloakUserCredential reconciliation
func (r *KeycloakUserCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakUserCredential"

	// Fetch the KeycloakUserCredential
	cred := &keycloakv1beta1.KeycloakUserCredential{}
	if err := r.Get(ctx, req.NamespacedName, cred); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakUserCredential")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, cred.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !cred.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cred, FinalizerName) {
			// Optionally delete the secret if we created it
			if cred.Status.SecretCreated {
				secret := &corev1.Secret{}
				secretName := types.NamespacedName{
					Name:      cred.Spec.UserSecret.SecretName,
					Namespace: cred.Namespace,
				}
				if err := r.Get(ctx, secretName, secret); err == nil {
					if err := r.Delete(ctx, secret); err != nil {
						log.Error(err, "failed to delete secret")
					}
				}
			}

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

	// Get the referenced user
	user, err := r.getReferencedUser(ctx, cred)
	if err != nil {
		RecordError(controllerName, "user_not_ready")
		return r.updateStatus(ctx, cred, false, "UserNotReady", err.Error(), "", 0)
	}

	// Check if user is ready
	if !user.Status.Ready || user.Status.UserID == "" {
		return r.updateStatus(ctx, cred, false, "UserNotReady", "Referenced user is not ready", "", 0)
	}

	// Get Keycloak client
	kc, realmName, err := r.getKeycloakClient(ctx, user)
	if err != nil {
		RecordError(controllerName, "instance_not_ready")
		return r.updateStatus(ctx, cred, false, "InstanceNotReady", err.Error(), "", 0)
	}

	// Get or create the secret
	secret, created, err := r.ensureSecret(ctx, cred, user)
	if err != nil {
		RecordError(controllerName, "secret_error")
		return r.updateStatus(ctx, cred, false, "SecretError", err.Error(), "", 0)
	}

	// Get password from secret
	passwordKey := cred.Spec.UserSecret.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	password, ok := secret.Data[passwordKey]
	if !ok || len(password) == 0 {
		RecordError(controllerName, "invalid_secret")
		return r.updateStatus(ctx, cred, false, "InvalidSecret", fmt.Sprintf("Secret missing key: %s", passwordKey), "", 0)
	}

	// Calculate password hash to detect changes
	passwordHash := hashPassword(password)

	// Check if password has changed since last sync
	needsSync := cred.Status.PasswordHash != passwordHash ||
		cred.Status.ObservedGeneration != cred.Generation ||
		!cred.Status.Ready

	if needsSync {
		// Set the password in Keycloak
		if err := kc.SetPassword(ctx, realmName, user.Status.UserID, string(password), false); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, cred, false, "PasswordSyncFailed", fmt.Sprintf("Failed to set password: %v", err), "", 0)
		}
		log.Info("password synchronized", "user", user.Name, "secret", secret.Name, "hashChanged", cred.Status.PasswordHash != passwordHash)
	} else {
		log.V(1).Info("password unchanged, skipping sync", "user", user.Name)
	}

	// Update status - only set SecretCreated to true if we created it,
	// but don't reset it to false if it was already created previously
	if created {
		cred.Status.SecretCreated = true
	}
	cred.Status.ResourcePath = user.Status.ResourcePath
	cred.Status.PasswordHash = passwordHash
	cred.Status.SecretResourceVersion = secret.ResourceVersion
	return r.updateStatus(ctx, cred, true, "Ready", "Credentials synchronized", passwordHash, cred.Generation)
}

func (r *KeycloakUserCredentialReconciler) getReferencedUser(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential) (*keycloakv1beta1.KeycloakUser, error) {
	userNamespace := cred.Namespace
	if cred.Spec.UserRef.Namespace != nil {
		userNamespace = *cred.Spec.UserRef.Namespace
	}

	user := &keycloakv1beta1.KeycloakUser{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cred.Spec.UserRef.Name,
		Namespace: userNamespace,
	}, user); err != nil {
		return nil, fmt.Errorf("failed to get KeycloakUser %s/%s: %w", userNamespace, cred.Spec.UserRef.Name, err)
	}

	return user, nil
}

func (r *KeycloakUserCredentialReconciler) getKeycloakClient(ctx context.Context, user *keycloakv1beta1.KeycloakUser) (*keycloak.Client, string, error) {
	// Check if using cluster realm ref
	if user.Spec.ClusterRealmRef != nil {
		return r.getKeycloakClientFromClusterRealm(ctx, user.Spec.ClusterRealmRef.Name)
	}

	// Use namespaced realm ref
	if user.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("user %s has no realmRef or clusterRealmRef", user.Name)
	}

	// Get the realm reference
	realmNamespace := user.Namespace
	if user.Spec.RealmRef.Namespace != nil {
		realmNamespace = *user.Spec.RealmRef.Namespace
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      user.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm: %w", err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realm.Name)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get the instance reference
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
		return nil, "", fmt.Errorf("failed to get KeycloakInstance: %w", err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instance.Name)
	}

	// Get the Keycloak client
	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("failed to get Keycloak client")
	}

	return kc, realmDef.Realm, nil
}

func (r *KeycloakUserCredentialReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, error) {
	// Get the ClusterKeycloakRealm
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	// Get realm name
	realmName := clusterRealm.Status.RealmName
	if realmName == "" {
		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(clusterRealm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", fmt.Errorf("failed to parse cluster realm definition: %w", err)
		}
		realmName = realmDef.Realm
	}

	// Get Keycloak client from cluster instance
	if clusterRealm.Spec.ClusterInstanceRef != nil {
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, clusterInstance)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, nil
	}

	// Use namespaced instance ref
	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceName := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, nil
}

func (r *KeycloakUserCredentialReconciler) ensureSecret(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential, user *keycloakv1beta1.KeycloakUser) (*corev1.Secret, bool, error) {
	secretName := types.NamespacedName{
		Name:      cred.Spec.UserSecret.SecretName,
		Namespace: cred.Namespace,
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, secretName, secret)

	if err == nil {
		// Secret exists
		return secret, false, nil
	}

	if !errors.IsNotFound(err) {
		return nil, false, err
	}

	// Secret doesn't exist
	if !cred.Spec.UserSecret.Create {
		return nil, false, fmt.Errorf("secret %s not found and create=false", secretName)
	}

	// Create the secret
	password, err := r.generatePassword(cred.Spec.UserSecret.PasswordPolicy)
	if err != nil {
		return nil, false, fmt.Errorf("failed to generate password: %w", err)
	}

	usernameKey := cred.Spec.UserSecret.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := cred.Spec.UserSecret.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	// Get username from user definition (simplified - just use user.Name)
	username := user.Name

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cred.Spec.UserSecret.SecretName,
			Namespace: cred.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "keycloak-operator",
				"keycloak.hostzero.com/user":   user.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			usernameKey: []byte(username),
			passwordKey: []byte(password),
		},
	}

	// Add email if key is specified
	if cred.Spec.UserSecret.EmailKey != "" {
		// Would need to extract email from user definition
		secret.Data[cred.Spec.UserSecret.EmailKey] = []byte("")
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(cred, secret, r.Scheme); err != nil {
		return nil, false, err
	}

	if err := r.Create(ctx, secret); err != nil {
		return nil, false, err
	}

	return secret, true, nil
}

func (r *KeycloakUserCredentialReconciler) generatePassword(policy *keycloakv1beta1.PasswordPolicySpec) (string, error) {
	length := 24
	if policy != nil && policy.Length > 0 {
		length = policy.Length
	}

	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64 and truncate
	password := base64.RawURLEncoding.EncodeToString(bytes)
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

func (r *KeycloakUserCredentialReconciler) updateStatus(ctx context.Context, cred *keycloakv1beta1.KeycloakUserCredential, ready bool, status, message, passwordHash string, observedGeneration int64) (ctrl.Result, error) {
	cred.Status.Ready = ready
	cred.Status.Status = status
	cred.Status.Message = message
	if passwordHash != "" {
		cred.Status.PasswordHash = passwordHash
	}
	if observedGeneration > 0 {
		cred.Status.ObservedGeneration = observedGeneration
	}

	if err := r.Status().Update(ctx, cred); err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
	}

	// Requeue after 5 minutes for periodic sync check
	return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
}

// hashPassword creates a SHA256 hash of the password for change detection
func hashPassword(password []byte) string {
	hash := sha256.Sum256(password)
	return hex.EncodeToString(hash[:])
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakUserCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakUserCredential{}).
		Owns(&corev1.Secret{}).
		// Watch for changes to referenced Secrets (for existing secrets not created by us)
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findCredentialsForSecret),
		).
		Complete(r)
}

// findCredentialsForSecret maps a Secret to the KeycloakUserCredential that references it
func (r *KeycloakUserCredentialReconciler) findCredentialsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	log := log.FromContext(ctx)

	// List all KeycloakUserCredentials in the same namespace
	var credList keycloakv1beta1.KeycloakUserCredentialList
	if err := r.List(ctx, &credList, client.InNamespace(secret.Namespace)); err != nil {
		log.Error(err, "failed to list KeycloakUserCredentials")
		return nil
	}

	var requests []reconcile.Request
	for _, cred := range credList.Items {
		// Check if this credential references this secret
		if cred.Spec.UserSecret.SecretName == secret.Name {
			log.V(1).Info("secret changed, triggering reconcile",
				"secret", secret.Name,
				"credential", cred.Name,
				"resourceVersion", secret.ResourceVersion,
			)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cred.Name,
					Namespace: cred.Namespace,
				},
			})
		}
	}

	return requests
}
