package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// KeycloakClientScopeReconciler reconciles a KeycloakClientScope object
type KeycloakClientScopeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientManager *keycloak.ClientManager
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclientscopes/finalizers,verbs=update

// Reconcile handles KeycloakClientScope reconciliation
func (r *KeycloakClientScopeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()
	controllerName := "KeycloakClientScope"

	// Fetch the KeycloakClientScope
	clientScope := &keycloakv1beta1.KeycloakClientScope{}
	if err := r.Get(ctx, req.NamespacedName, clientScope); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakClientScope")
		RecordReconcile(controllerName, false, time.Since(startTime).Seconds())
		RecordError(controllerName, "fetch_error")
		return ctrl.Result{}, err
	}

	// Defer metrics recording
	defer func() {
		RecordReconcile(controllerName, clientScope.Status.Ready, time.Since(startTime).Seconds())
	}()

	// Handle deletion
	if !clientScope.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(clientScope, FinalizerName) {
			// Delete client scope from Keycloak unless preserve annotation is set
			if ShouldPreserveResource(clientScope) {
				log.Info("preserving client scope in Keycloak due to annotation", "annotation", PreserveResourceAnnotation)
			} else if err := r.deleteClientScope(ctx, clientScope); err != nil {
				log.Error(err, "failed to delete client scope from Keycloak")
			}

			controllerutil.RemoveFinalizer(clientScope, FinalizerName)
			if err := r.Update(ctx, clientScope); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(clientScope, FinalizerName) {
		controllerutil.AddFinalizer(clientScope, FinalizerName)
		if err := r.Update(ctx, clientScope); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, clientScope)
	if err != nil {
		RecordError(controllerName, "realm_not_ready")
		return r.updateStatus(ctx, clientScope, false, "RealmNotReady", err.Error(), "")
	}

	// Parse client scope definition to extract name
	var scopeDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(clientScope.Spec.Definition.Raw, &scopeDef); err != nil {
		RecordError(controllerName, "invalid_definition")
		return r.updateStatus(ctx, clientScope, false, "InvalidDefinition", fmt.Sprintf("Failed to parse client scope definition: %v", err), "")
	}

	// Keycloak's client scope PUT silently discards protocolMappers, so they are
	// only ever managed through KeycloakProtocolMapper.
	if err := rejectDefinitionKey(clientScope.Spec.Definition.Raw, "protocolMappers", "KeycloakProtocolMapper"); err != nil {
		RecordError(controllerName, "unsupported_definition_field")
		return r.updateStatus(ctx, clientScope, false, UnsupportedDefinitionFieldReason, err.Error(), "")
	}

	// Resolve the client scope name from spec.name.
	scopeName, err := resolveIdentifier("name", clientScope.Spec.Name, scopeDef.Name)
	if err != nil {
		RecordError(controllerName, "invalid_identifier")
		return r.updateStatus(ctx, clientScope, false, InvalidIdentifierReason, err.Error(), "")
	}
	scopeDef.Name = scopeName
	clientScope.Status.ClientScopeName = scopeName

	// Prepare definition JSON with name set
	definition := setFieldInDefinition(clientScope.Spec.Definition.Raw, "name", scopeDef.Name)

	// Check if client scope exists by name
	existingScopes, err := kc.GetClientScopes(ctx, realmName)
	var existingScope *keycloak.ClientScopeRepresentation
	if err == nil {
		for i := range existingScopes {
			if existingScopes[i].Name != nil && *existingScopes[i].Name == scopeDef.Name {
				existingScope = &existingScopes[i]
				break
			}
		}
	}

	var scopeID string
	if existingScope == nil {
		// Client scope doesn't exist, create it
		log.Info("creating client scope", "name", scopeDef.Name, "realm", realmName)
		scopeID, err = kc.CreateClientScope(ctx, realmName, definition)
		if err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, clientScope, false, "CreateFailed", fmt.Sprintf("Failed to create client scope: %v", err), "")
		}
		log.Info("client scope created successfully", "name", scopeDef.Name, "id", scopeID)
	} else {
		// Client scope exists, update it
		scopeID = *existingScope.ID
		definition = mergeIDIntoDefinition(definition, existingScope.ID)

		log.Info("updating client scope", "name", scopeDef.Name, "realm", realmName)
		if err := kc.UpdateClientScope(ctx, realmName, scopeID, definition); err != nil {
			RecordError(controllerName, "keycloak_api_error")
			return r.updateStatus(ctx, clientScope, false, "UpdateFailed", fmt.Sprintf("Failed to update client scope: %v", err), scopeID)
		}
		log.Info("client scope updated successfully", "name", scopeDef.Name)
	}

	// Update status
	clientScope.Status.ResourcePath = fmt.Sprintf("/admin/realms/%s/client-scopes/%s", realmName, scopeID)
	return r.updateStatus(ctx, clientScope, true, "Ready", "Client scope synchronized", scopeID)
}

func (r *KeycloakClientScopeReconciler) getKeycloakClientAndRealm(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope) (*keycloak.Client, string, error) {
	res, err := ResolveRealm(ctx, r.Client, r.ClientManager, clientScope.Namespace, clientScope.Spec.RealmRef, clientScope.Spec.ClusterRealmRef)
	if err != nil {
		return nil, "", err
	}
	return res.Client, res.RealmName, nil
}

func (r *KeycloakClientScopeReconciler) deleteClientScope(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope) error {
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, clientScope)
	if err != nil {
		return err
	}

	// Use spec.name so deletion targets the synchronized scope. Empty means
	// never synchronized (unmigrated object).
	scopeName := identifierValue(clientScope.Spec.Name)
	if scopeName == "" {
		return nil
	}

	// Find scope by name
	scopes, err := kc.GetClientScopes(ctx, realmName)
	if err != nil {
		return err
	}

	for _, s := range scopes {
		if s.Name != nil && *s.Name == scopeName {
			return kc.DeleteClientScope(ctx, realmName, *s.ID)
		}
	}

	return nil // Scope doesn't exist
}

func (r *KeycloakClientScopeReconciler) updateStatus(ctx context.Context, clientScope *keycloakv1beta1.KeycloakClientScope, ready bool, status, message, scopeID string) (ctrl.Result, error) {
	clientScope.Status.Ready = ready
	clientScope.Status.Status = status
	clientScope.Status.Message = message

	clientScope.Status.Conditions = setReadyCondition(clientScope.Status.Conditions, ready, status, message)

	return writeStatusIfChanged(ctx, r.Client, clientScope, ready)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakClientScopeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClientScope{}).
		Complete(r)
}
