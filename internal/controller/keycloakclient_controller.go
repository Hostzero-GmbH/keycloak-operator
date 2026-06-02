package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

	// Extract client scope assignments before sending to Keycloak.
	// Keycloak's client REST API ignores these fields — they require dedicated
	// scope-assignment endpoints, which we call after create/update.
	desiredDefaultScopes, hasDefaultScopes := extractStringSliceFromDefinition(definition, "defaultClientScopes")
	desiredOptionalScopes, hasOptionalScopes := extractStringSliceFromDefinition(definition, "optionalClientScopes")
	definition = removeFieldFromDefinition(definition, "defaultClientScopes")
	definition = removeFieldFromDefinition(definition, "optionalClientScopes")

	// Resolve authentication flow binding aliases to UUIDs
	definition, err = resolveFlowBindingAliases(ctx, kc, realmName, definition)
	if err != nil {
		RecordError(controllerName, "flow_alias_error")
		return r.updateStatus(ctx, kcClient, false, "FlowAliasResolutionFailed", fmt.Sprintf("Failed to resolve flow alias: %v", err), "", instanceRef, realmRef)
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
		// Client exists — check if update is needed
		clientUUID = *existingClient.ID
		definition = mergeIDIntoDefinition(definition, existingClient.ID)

		// Fetch current state from Keycloak for drift detection
		currentRaw, fetchErr := kc.GetClientRaw(ctx, realmName, clientUUID)

		needsUpdate := true
		if fetchErr != nil {
			log.Error(fetchErr, "failed to fetch current client state, falling through to update")
		} else if currentRaw != nil {
			needsUpdate = !definitionsMatch(definition, currentRaw)
		}

		if needsUpdate {
			log.Info("updating client", "clientId", clientDef.ClientID, "realm", realmName)
			if err := kc.UpdateClient(ctx, realmName, clientUUID, definition); err != nil {
				RecordError(controllerName, "keycloak_api_error")
				return r.updateStatus(ctx, kcClient, false, "UpdateFailed", fmt.Sprintf("Failed to update client: %v", err), clientUUID, instanceRef, realmRef)
			}
			log.Info("client updated successfully", "clientId", clientDef.ClientID)
		} else {
			log.V(1).Info("client already in sync, skipping update", "clientId", clientDef.ClientID)
		}
	}

	// Sync default/optional client scope assignments
	if err := r.syncClientScopes(ctx, kc, realmName, clientUUID,
		desiredDefaultScopes, hasDefaultScopes,
		desiredOptionalScopes, hasOptionalScopes); err != nil {
		RecordError(controllerName, "scope_sync_error")
		return r.updateStatus(ctx, kcClient, false, "ScopeSyncFailed", fmt.Sprintf("Failed to sync client scopes: %v", err), clientUUID, instanceRef, realmRef)
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
	realmName := types.NamespacedName{
		Name:      kcClient.Spec.RealmRef.Name,
		Namespace: kcClient.Namespace,
	}
	realmRef.RealmRef = fmt.Sprintf("%s/%s", kcClient.Namespace, kcClient.Spec.RealmRef.Name)

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

	// Get Keycloak client from realm's instance
	kc, _, err := GetKeycloakClientFromRealmInstance(ctx, r.Client, r.ClientManager, realm)
	if err != nil {
		return nil, "", instanceRef, realmRef, err
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
// - If secret exists but key is missing and the client is public: returns ("", false, nil)
// - If secret exists but key is missing on a non-public client: returns ("", false, error)
// - If secret doesn't exist and create=true: returns ("", true, nil) - will be created after Keycloak generates
// - If secret doesn't exist and create=false: returns ("", false, error)
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
			// Public clients legitimately have no client-secret key; the Secret
			// is only used as a holder for the client-id. Treat the missing key
			// as "no pre-existing secret to inject" rather than an error.
			if isPublicClient(kcClient) {
				return "", false, nil
			}
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

// isPublicClient reports whether the KeycloakClient spec marks the client as
// public. A public client has no OAuth client_secret, so the K8s Secret will
// only carry the client-id key.
func isPublicClient(kcClient *keycloakv1beta1.KeycloakClient) bool {
	if kcClient.Spec.Definition == nil || len(kcClient.Spec.Definition.Raw) == 0 {
		return false
	}
	var parsed struct {
		PublicClient *bool `json:"publicClient"`
	}
	if err := json.Unmarshal(kcClient.Spec.Definition.Raw, &parsed); err != nil {
		return false
	}
	return parsed.PublicClient != nil && *parsed.PublicClient
}

func (r *KeycloakClientReconciler) syncClientSecret(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient, kc *keycloak.Client, realmName, clientUUID string) error {
	// Get client secret from Keycloak. For public clients this returns an empty
	// string — we still want to materialise a Secret so consumers can pick up
	// the client-id via envFrom/secretKeyRef, matching the legacy operator's
	// behaviour.
	secretValue, err := kc.GetClientSecret(ctx, realmName, clientUUID)
	if err != nil {
		return fmt.Errorf("failed to get client secret: %w", err)
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
		// Only emit the client-secret key when Keycloak returned one (i.e. confidential client).
		if secretValue != "" {
			secret.Data[clientSecretKey] = []byte(secretValue)
		}
		secret.Type = corev1.SecretTypeOpaque
		return controllerutil.SetControllerReference(kcClient, secret, r.Scheme)
	})

	return err
}

// syncClientScopes reconciles the default and optional client scope assignments
// for a Keycloak client. It only acts when the corresponding field was explicitly
// present in the definition JSON (even an empty array means "remove all").
// If the field was absent, the operator leaves Keycloak's assignments untouched.
func (r *KeycloakClientReconciler) syncClientScopes(
	ctx context.Context, kc *keycloak.Client, realmName, clientUUID string,
	desiredDefault []string, hasDefault bool,
	desiredOptional []string, hasOptional bool,
) error {
	log := log.FromContext(ctx)

	if !hasDefault && !hasOptional {
		return nil
	}

	allScopes, err := kc.GetClientScopes(ctx, realmName)
	if err != nil {
		return fmt.Errorf("failed to list realm client scopes: %w", err)
	}
	scopeNameToID := make(map[string]string, len(allScopes))
	for _, s := range allScopes {
		if s.Name != nil && s.ID != nil {
			scopeNameToID[*s.Name] = *s.ID
		}
	}

	if hasDefault {
		if err := r.reconcileScopeAssignments(ctx, log, kc, realmName, clientUUID, "default", desiredDefault, scopeNameToID,
			kc.GetClientDefaultScopes, kc.AddClientDefaultScope, kc.RemoveClientDefaultScope); err != nil {
			return fmt.Errorf("failed to sync default client scopes: %w", err)
		}
	}

	if hasOptional {
		if err := r.reconcileScopeAssignments(ctx, log, kc, realmName, clientUUID, "optional", desiredOptional, scopeNameToID,
			kc.GetClientOptionalScopes, kc.AddClientOptionalScope, kc.RemoveClientOptionalScope); err != nil {
			return fmt.Errorf("failed to sync optional client scopes: %w", err)
		}
	}

	return nil
}

func (r *KeycloakClientReconciler) reconcileScopeAssignments(
	ctx context.Context,
	log logr.Logger,
	kc *keycloak.Client,
	realmName, clientUUID, scopeType string,
	desiredNames []string,
	scopeNameToID map[string]string,
	getCurrent func(ctx context.Context, realm, clientUUID string) ([]keycloak.ClientScopeRepresentation, error),
	addScope func(ctx context.Context, realm, clientUUID, scopeID string) error,
	removeScope func(ctx context.Context, realm, clientUUID, scopeID string) error,
) error {
	current, err := getCurrent(ctx, realmName, clientUUID)
	if err != nil {
		return fmt.Errorf("failed to get current %s scopes: %w", scopeType, err)
	}

	currentByName := make(map[string]string, len(current))
	for _, s := range current {
		if s.Name != nil && s.ID != nil {
			currentByName[*s.Name] = *s.ID
		}
	}

	desiredSet := make(map[string]struct{}, len(desiredNames))
	for _, name := range desiredNames {
		desiredSet[name] = struct{}{}
	}

	// Remove scopes not in the desired list
	for name, id := range currentByName {
		if _, wanted := desiredSet[name]; !wanted {
			log.Info("removing "+scopeType+" client scope", "scope", name, "clientUUID", clientUUID)
			if err := removeScope(ctx, realmName, clientUUID, id); err != nil {
				return fmt.Errorf("failed to remove %s scope %q: %w", scopeType, name, err)
			}
		}
	}

	// Add scopes that are desired but not yet assigned
	for _, name := range desiredNames {
		if _, exists := currentByName[name]; exists {
			continue
		}
		id, ok := scopeNameToID[name]
		if !ok {
			return fmt.Errorf("%s scope %q does not exist in realm %q", scopeType, name, realmName)
		}
		log.Info("adding "+scopeType+" client scope", "scope", name, "clientUUID", clientUUID)
		if err := addScope(ctx, realmName, clientUUID, id); err != nil {
			return fmt.Errorf("failed to add %s scope %q: %w", scopeType, name, err)
		}
	}

	return nil
}

// extractStringSliceFromDefinition extracts a string slice field from the
// definition JSON. It returns the slice and a boolean indicating whether the
// field was present in the JSON (an explicit null or missing key → false).
func extractStringSliceFromDefinition(definition []byte, field string) ([]string, bool) {
	var defMap map[string]json.RawMessage
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return nil, false
	}

	raw, exists := defMap[field]
	if !exists {
		return nil, false
	}

	// Explicit null in JSON
	if string(raw) == "null" {
		return nil, false
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, false
	}
	return values, true
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
	// Determine desired condition status
	desiredConditionStatus := metav1.ConditionFalse
	if ready {
		desiredConditionStatus = metav1.ConditionTrue
	}

	// Check if status actually changed
	statusChanged := kcClient.Status.Ready != ready ||
		kcClient.Status.Status != status ||
		kcClient.Status.Message != message ||
		kcClient.Status.ClientUUID != clientUUID

	conditionChanged := true
	for _, c := range kcClient.Status.Conditions {
		if c.Type == "Ready" && c.Status == desiredConditionStatus && c.Reason == status && c.Message == message {
			conditionChanged = false
			break
		}
	}

	// Check if observed generation changed
	generationChanged := ready && kcClient.Status.ObservedGeneration != kcClient.Generation

	if !statusChanged && !conditionChanged && !generationChanged {
		// Nothing changed, just requeue without writing to API
		if ready {
			return ctrl.Result{RequeueAfter: GetSyncPeriod()}, nil
		}
		return ctrl.Result{RequeueAfter: ErrorRequeueDelay}, nil
	}

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
		Status:             desiredConditionStatus,
		Reason:             status,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Update or add condition
	found := false
	for i, c := range kcClient.Status.Conditions {
		if c.Type == "Ready" {
			// Preserve LastTransitionTime if status didn't change
			if c.Status == desiredConditionStatus {
				condition.LastTransitionTime = c.LastTransitionTime
			}
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

// definitionsMatch compares desired definition against the current state in Keycloak.
// Only fields present in the desired definition are compared — extra fields returned
// by Keycloak (like access, authenticationFlowBindingOverrides, etc.) are ignored.
// Array fields (e.g. defaultClientScopes, redirectUris) are compared as unordered sets
// because Keycloak may return them in arbitrary order.
func definitionsMatch(desired, current json.RawMessage) bool {
	var desiredMap, currentMap map[string]interface{}
	if err := json.Unmarshal(desired, &desiredMap); err != nil {
		return false
	}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return false
	}

	for key, desiredVal := range desiredMap {
		// defaultClientScopes and optionalClientScopes are reconciled via dedicated
		// scope-assignment endpoints (see syncClientScopes); the client representation
		// PUT/GET round-trip doesn't faithfully preserve them, so skip in the diff.
		if key == "defaultClientScopes" || key == "optionalClientScopes" {
			continue
		}
		currentVal, exists := currentMap[key]
		if !exists {
			return false
		}
		if !valuesMatch(desiredVal, currentVal) {
			return false
		}
	}
	return true
}

// valuesMatch compares two values, treating JSON arrays as unordered sets of strings
// when all elements are strings. This prevents false diffs caused by Keycloak returning
// array fields like defaultClientScopes in non-deterministic order.
func valuesMatch(desired, current interface{}) bool {
	desiredArr, dIsArr := toStringSlice(desired)
	currentArr, cIsArr := toStringSlice(current)
	if dIsArr && cIsArr {
		if len(desiredArr) != len(currentArr) {
			return false
		}
		sort.Strings(desiredArr)
		sort.Strings(currentArr)
		for i := range desiredArr {
			if desiredArr[i] != currentArr[i] {
				return false
			}
		}
		return true
	}

	// Maps: subset comparison (e.g. attributes — CR defines a subset, KC adds defaults)
	desiredMap, dIsMap := desired.(map[string]interface{})
	currentMap, cIsMap := current.(map[string]interface{})
	if dIsMap && cIsMap {
		for k, dv := range desiredMap {
			cv, exists := currentMap[k]
			if !exists {
				return false
			}
			if !valuesMatch(dv, cv) {
				return false
			}
		}
		return true
	}

	// Arrays of objects (e.g. protocolMappers): match by "name" field, require same
	// length so that extra objects in Keycloak (e.g. an orphaned protocolMapper that
	// the CR no longer declares) are detected as drift and removed by the PUT.
	// Within each matched object, fields are still subset-compared because Keycloak
	// adds fields the CR omits (id, consentRequired, ...).
	desiredObjArr, dIsObjArr := toObjectSlice(desired)
	currentObjArr, cIsObjArr := toObjectSlice(current)
	if dIsObjArr && cIsObjArr {
		if len(desiredObjArr) != len(currentObjArr) {
			return false
		}
		for _, dObj := range desiredObjArr {
			dName, _ := dObj["name"].(string)
			found := false
			for _, cObj := range currentObjArr {
				cName, _ := cObj["name"].(string)
				if dName != "" && dName == cName {
					// Subset compare: all desired fields must match
					match := true
					for k, dv := range dObj {
						cv, exists := cObj[k]
						if !exists {
							match = false
							break
						}
						if !valuesMatch(dv, cv) {
							match = false
							break
						}
					}
					if match {
						found = true
					}
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}

	dj, err1 := json.Marshal(desired)
	cj, err2 := json.Marshal(current)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(dj) == string(cj)
}

// toStringSlice checks if a value is a JSON array of strings and returns it as []string.
func toStringSlice(v interface{}) ([]string, bool) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, s)
	}
	return result, true
}

// toObjectSlice checks if a value is a JSON array of objects and returns it as []map[string]interface{}.
func toObjectSlice(v interface{}) ([]map[string]interface{}, bool) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, false
	}
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		result = append(result, obj)
	}
	return result, true
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClient{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
