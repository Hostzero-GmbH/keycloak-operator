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
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// KeycloakClientReconciler reconciles a KeycloakClient object
type KeycloakClientReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakrealms,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles KeycloakClient reconciliation
func (r *KeycloakClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakClient"

	// Fetch the KeycloakClient
	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, req.NamespacedName, kcClient); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakClient")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, kcClient.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !kcClient.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(kcClient, FinalizerName) {
			// Delete client from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(kcClient) {
				log.Info("preserving client in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteClient(ctx, kcClient); err != nil {
				log.Error(err, "failed to delete client from Keycloak")
				// Continue with finalizer removal even on error
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(kcClient, FinalizerName)
			if err := r.Update(ctx, kcClient); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kcClient, FinalizerName) {
		controllerutil.AddFinalizer(kcClient, FinalizerName)
		if err := r.Update(ctx, kcClient); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, instanceRef, realmRef, err := r.getKeycloakClientAndRealm(ctx, kcClient)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, kcClient, false, "RealmNotReady", err.Error(), "", instanceRef, realmRef)
	}

	// Parse client definition to extract clientId
	var clientDef struct {
		ID       string `json:"id,omitempty"`
		ClientID string `json:"clientId,omitempty"`
	}
	if kcClient.Spec.Definition != nil {
		if err := json.Unmarshal(kcClient.Spec.Definition.Raw, &clientDef); err != nil {
			RecordError(controllerName, "invalid_definition")
			return r.updateStatus(ctx, kcClient, false, "InvalidDefinition", fmt.Sprintf("Failed to parse client definition: %v", err), "", instanceRef, realmRef)
		}
	}

	// Set clientId from spec or use the one in definition
	if kcClient.Spec.ClientId != nil && *kcClient.Spec.ClientId != "" {
		clientDef.ClientID = *kcClient.Spec.ClientId
	}

	// Ensure clientId is set
	if clientDef.ClientID == "" {
		// Default to metadata.name
		clientDef.ClientID = kcClient.Name
	}

	// Prepare definition JSON with clientId set
	var definition []byte
	if kcClient.Spec.Definition != nil {
		definition = kcClient.Spec.Definition.Raw
	}
	if definition == nil {
		definition = []byte("{}")
	}
	definition = setFieldInDefinition(definition, "clientId", clientDef.ClientID)

	// Handle client secret - check if we should use a pre-existing secret
	var preExistingSecret string
	var secretNeedsCreation bool
	if kcClient.Spec.ClientSecretRef != nil {
		secret, needsCreation, err := r.ensureClientSecret(ctx, kcClient)
		if err != nil {
			RecordError(controllerName, "secret_error")
			return r.updateStatus(ctx, kcClient, false, "SecretError", err.Error(), "", instanceRef, realmRef)
		}
		preExistingSecret = secret
		secretNeedsCreation = needsCreation

		// If we have a pre-existing secret value, inject it into the definition
		if preExistingSecret != "" {
			definition = setFieldInDefinition(definition, "secret", preExistingSecret)
		}
	}

	// Check if client exists
	existingClient, err := kc.GetClientByClientID(ctx, realmName, clientDef.ClientID)

	var clientUUID string
	if err != nil {
		// Client doesn't exist, create it
		log.Info("creating client", "clientId", clientDef.ClientID, "realm", realmName)
		clientUUID, err = kc.CreateClient(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, kcClient, false, "CreateFailed", fmt.Sprintf("Failed to create client: %v", err), "", instanceRef, realmRef)
		}
		log.Info("client created successfully", "clientId", clientDef.ClientID, "uuid", clientUUID)
	} else {
		// Client exists, update it
		clientUUID = *existingClient.ID
		definition = mergeIDIntoDefinition(definition, existingClient.ID)

		log.Info("updating client", "clientId", clientDef.ClientID, "realm", realmName)
		if err := kc.UpdateClient(ctx, realmName, clientUUID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, kcClient, false, "UpdateFailed", fmt.Sprintf("Failed to update client: %v", err), clientUUID, instanceRef, realmRef)
		}
		log.Info("client updated successfully", "clientId", clientDef.ClientID)
	}

	// Handle client secret sync - only if secretNeedsCreation (no pre-existing secret)
	if kcClient.Spec.ClientSecretRef != nil && secretNeedsCreation {
		if err := r.syncClientSecret(ctx, kcClient, kc, realmName, clientUUID); err != nil {
			log.Error(err, "failed to sync client secret")
			RecordError(controllerName, "secret_sync_error")
			return r.updateStatus(ctx, kcClient, false, "SecretSyncFailed", err.Error(), clientUUID, instanceRef, realmRef)
		}
	}

	// Update status
	kcClient.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/clients/%s", realmName, clientUUID)
	return r.updateStatus(ctx, kcClient, true, "Ready", "Client synchronized", clientUUID, instanceRef, realmRef)
}

func (r *KeycloakClientReconciler) getKeycloakClientAndRealm(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient) (*keycloak.Client, string, *keycloakv1beta1.InstanceRef, *keycloakv1beta1.RealmRef, error) {
	instanceRef := &keycloakv1beta1.InstanceRef{}
	realmRef := &keycloakv1beta1.RealmRef{}

	// Check if using cluster realm ref
	if kcClient.Spec.ClusterRealmRef != nil {
		realmRef.ClusterRealmRef = kcClient.Spec.ClusterRealmRef.Name
		kc, realmName, instRef, err := r.getKeycloakClientFromClusterRealm(ctx, kcClient.Spec.ClusterRealmRef.Name)
		if err != nil {
			return nil, "", instRef, realmRef, err
		}
		return kc, realmName, instRef, realmRef, nil
	}

	// Use namespaced realm ref
	if kcClient.Spec.RealmRef == nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	// Get the realm reference
	realmNamespace := kcClient.Namespace
	if kcClient.Spec.RealmRef.Namespace != nil {
		realmNamespace = *kcClient.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      kcClient.Spec.RealmRef.Name,
		Namespace: realmNamespace,
	}
	realmRef.RealmRef = fmt.Sprintf("%s/%s", realmNamespace, kcClient.Spec.RealmRef.Name)

	// Get the KeycloakRealm
	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := r.Get(ctx, realmName, realm); err != nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	// Check if realm is ready
	if !realm.Status.Ready {
		return nil, "", instanceRef, realmRef, fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	// Get realm name from definition
	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("failed to parse realm definition: %w", err)
	}

	// Get instance reference from realm
	instanceNamespace := realm.Namespace
	if realm.Spec.InstanceRef.Namespace != nil {
		instanceNamespace = *realm.Spec.InstanceRef.Namespace
	}
	instanceName := types.NamespacedName{
		Name:      realm.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}
	instanceRef.InstanceRef = fmt.Sprintf("%s/%s", instanceNamespace, realm.Spec.InstanceRef.Name)

	// Get the KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	// Check if instance is ready
	if !instance.Status.Ready {
		return nil, "", instanceRef, realmRef, fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	// Get the Keycloak client from manager
	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", instanceRef, realmRef, fmt.Errorf("keycloak client not available for instance %s", instanceName)
	}

	return kc, realmDef.Realm, instanceRef, realmRef, nil
}

func (r *KeycloakClientReconciler) getKeycloakClientFromClusterRealm(ctx context.Context, clusterRealmName string) (*keycloak.Client, string, *keycloakv1beta1.InstanceRef, error) {
	instanceRef := &keycloakv1beta1.InstanceRef{}

	// Get the ClusterKeycloakRealm
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := r.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", instanceRef, fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", instanceRef, fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

	// Get realm name
	realmName := clusterRealm.Status.RealmName
	if realmName == "" {
		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(clusterRealm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", instanceRef, fmt.Errorf("failed to parse cluster realm definition: %w", err)
		}
		realmName = realmDef.Realm
	}

	// Get Keycloak client from cluster instance
	if clusterRealm.Spec.ClusterInstanceRef != nil {
		instanceRef.ClusterInstanceRef = clusterRealm.Spec.ClusterInstanceRef.Name

		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := r.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", instanceRef, fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", instanceRef, fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, r.Client, clusterInstance)
		if err != nil {
			return nil, "", instanceRef, fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := r.ClientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", instanceRef, fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, instanceRef, nil
	}

	// Use namespaced instance ref
	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", instanceRef, fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceName := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}
	instanceRef.InstanceRef = fmt.Sprintf("%s/%s", instanceName.Namespace, instanceName.Name)

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := r.Get(ctx, instanceName, instance); err != nil {
		return nil, "", instanceRef, fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", instanceRef, fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", instanceRef, fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := r.ClientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", instanceRef, fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, instanceRef, nil
}

// ensureClientSecret reads or creates the client secret.
// Returns: (secretValue, needsCreation, error)
// - If secret exists with the key: returns (value, false, nil)
// - If secret doesn't exist and create=true: returns ("", true, nil) - will be created after Keycloak generates
// - If secret doesn't exist and create=false: returns ("", false, error)
// - If secret exists but key is missing: returns ("", false, error)
func (r *KeycloakClientReconciler) ensureClientSecret(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient) (string, bool, error) {
	ref := kcClient.Spec.ClientSecretRef
	secretName := ref.Name
	secretKey := "client-secret"
	if ref.ClientSecretKey != nil && *ref.ClientSecretKey != "" {
		secretKey = *ref.ClientSecretKey
	}

	// Try to read existing secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: kcClient.Namespace,
	}, secret)

	if err == nil {
		// Secret exists - read the value
		value, ok := secret.Data[secretKey]
		if !ok {
			return "", false, fmt.Errorf("key %q not found in secret %q", secretKey, secretName)
		}
		return string(value), false, nil
	}

	if !errors.IsNotFound(err) {
		return "", false, fmt.Errorf("failed to get secret %q: %w", secretName, err)
	}

	// Secret doesn't exist
	create := ref.Create == nil || *ref.Create // default true
	if !create {
		return "", false, fmt.Errorf("secret %q not found and create=false", secretName)
	}

	// Will be created after Keycloak generates the secret
	return "", true, nil
}

func (r *KeycloakClientReconciler) syncClientSecret(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient, kc *keycloak.Client, realmName, clientUUID string) error {
	// Get client secret from Keycloak
	secretValue, err := kc.GetClientSecret(ctx, realmName, clientUUID)
	if err != nil {
		return fmt.Errorf("failed to get client secret: %w", err)
	}

	if secretValue == "" {
		return nil // No secret (public client)
	}

	// Get clientId from spec or definition
	var clientId string
	if kcClient.Spec.ClientId != nil && *kcClient.Spec.ClientId != "" {
		clientId = *kcClient.Spec.ClientId
	} else if kcClient.Spec.Definition != nil {
		var clientDef struct {
			ClientID string `json:"clientId"`
		}
		if err := json.Unmarshal(kcClient.Spec.Definition.Raw, &clientDef); err != nil {
			return fmt.Errorf("failed to parse client definition: %w", err)
		}
		clientId = clientDef.ClientID
	}
	if clientId == "" {
		clientId = kcClient.Name
	}

	// Determine secret keys
	clientIdKey := "client-id"
	clientSecretKey := "client-secret"
	if kcClient.Spec.ClientSecretRef.ClientIdKey != nil {
		clientIdKey = *kcClient.Spec.ClientSecretRef.ClientIdKey
	}
	if kcClient.Spec.ClientSecretRef.ClientSecretKey != nil {
		clientSecretKey = *kcClient.Spec.ClientSecretRef.ClientSecretKey
	}

	// Create or update the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kcClient.Spec.ClientSecretRef.Name,
			Namespace: kcClient.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Reset data to ensure only the specified keys exist
		secret.Data = make(map[string][]byte)
		secret.Data[clientIdKey] = []byte(clientId)
		secret.Data[clientSecretKey] = []byte(secretValue)
		secret.Type = corev1.SecretTypeOpaque
		return controllerutil.SetControllerReference(kcClient, secret, r.Scheme)
	})

	return err
}

func (r *KeycloakClientReconciler) deleteClient(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient) error {
	kc, realmName, _, _, err := r.getKeycloakClientAndRealm(ctx, kcClient)
	if err != nil {
		return err
	}

	// Get clientId from spec or definition
	var clientId string
	if kcClient.Spec.ClientId != nil && *kcClient.Spec.ClientId != "" {
		clientId = *kcClient.Spec.ClientId
	} else if kcClient.Spec.Definition != nil {
		var clientDef struct {
			ClientID string `json:"clientId"`
		}
		if err := json.Unmarshal(kcClient.Spec.Definition.Raw, &clientDef); err != nil {
			return fmt.Errorf("failed to parse client definition: %w", err)
		}
		clientId = clientDef.ClientID
	}
	if clientId == "" {
		clientId = kcClient.Name
	}

	// Find client by clientId
	existingClient, err := kc.GetClientByClientID(ctx, realmName, clientId)
	if err != nil {
		return nil // Client doesn't exist
	}

	return kc.DeleteClient(ctx, realmName, *existingClient.ID)
}

func (r *KeycloakClientReconciler) updateStatus(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient, ready bool, status, message, clientUUID string, instanceRef *keycloakv1beta1.InstanceRef, realmRef *keycloakv1beta1.RealmRef) (ctrl.Result, error) {
	kcClient.Status.Ready = ready
	kcClient.Status.Status = status
	kcClient.Status.Message = message
	kcClient.Status.ClientUUID = clientUUID
	kcClient.Status.Instance = instanceRef
	kcClient.Status.Realm = realmRef

	// Track observed generation to detect spec changes
	if ready {
		kcClient.Status.ObservedGeneration = kcClient.Generation
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
	for i, c := range kcClient.Status.Conditions {
		if c.Type == "Ready" {
			kcClient.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		kcClient.Status.Conditions = append(kcClient.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, kcClient); err != nil {
		return ctrl.Result{}, err
	}

	if ready {
		return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
	}
	return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClient{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
