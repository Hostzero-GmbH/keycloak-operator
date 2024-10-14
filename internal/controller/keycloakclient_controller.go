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

// KeycloakClientReconciler reconciles a KeycloakClient object
type KeycloakClientReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakclients/finalizers,verbs=update

// Reconcile handles KeycloakClient reconciliation
func (r *KeycloakClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the KeycloakClient
	kcClient := &keycloakv1beta1.KeycloakClient{}
	if err := r.Get(ctx, req.NamespacedName, kcClient); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakClient")
		return ctrl.Result{}, err
	}

	// Get Keycloak client and realm info
	kc, realmName, err := r.getKeycloakClientAndRealm(ctx, kcClient)
	if err != nil {
		log.Error(err, "failed to get keycloak client")
		return r.updateStatus(ctx, kcClient, false, "RealmNotReady", err.Error())
	}

	// Parse client definition
	var clientDef struct {
		ClientID string `json:"clientId"`
	}
	if err := json.Unmarshal(kcClient.Spec.Definition.Raw, &clientDef); err != nil {
		log.Error(err, "failed to parse client definition")
		return r.updateStatus(ctx, kcClient, false, "InvalidDefinition", err.Error())
	}

	if clientDef.ClientID == "" {
		return r.updateStatus(ctx, kcClient, false, "InvalidDefinition", "clientId is required in definition")
	}

	// Check if client exists
	existingClient, err := kc.GetClientByClientID(ctx, realmName, clientDef.ClientID)
	if err != nil {
		// Client doesn't exist, create it
		log.Info("creating client", "clientId", clientDef.ClientID)
		_, err = kc.CreateClient(ctx, realmName, kcClient.Spec.Definition.Raw)
		if err != nil {
			log.Error(err, "failed to create client")
			return r.updateStatus(ctx, kcClient, false, "CreateFailed", err.Error())
		}
	} else {
		// Client exists, update it
		log.Info("updating client", "clientId", clientDef.ClientID)
		if err := kc.UpdateClient(ctx, realmName, *existingClient.ID, kcClient.Spec.Definition.Raw); err != nil {
			log.Error(err, "failed to update client")
			return r.updateStatus(ctx, kcClient, false, "UpdateFailed", err.Error())
		}
	}

	log.Info("client reconciled", "clientId", clientDef.ClientID)
	return r.updateStatus(ctx, kcClient, true, "Ready", "Client synchronized")
}

func (r *KeycloakClientReconciler) updateStatus(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient, ready bool, status, message string) (ctrl.Result, error) {
	kcClient.Status.Ready = ready
	kcClient.Status.Status = status
	kcClient.Status.Message = message

	if err := r.Status().Update(ctx, kcClient); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeycloakClientReconciler) getKeycloakClientAndRealm(ctx context.Context, kcClient *keycloakv1beta1.KeycloakClient) (*keycloak.Client, string, error) {
	// Get the realm
	realmNamespace := kcClient.Namespace
	if kcClient.Spec.RealmRef.Namespace != nil {
		realmNamespace = *kcClient.Spec.RealmRef.Namespace
	}
	realmName := types.NamespacedName{
		Name:      kcClient.Spec.RealmRef.Name,
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
func (r *KeycloakClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakClient{}).
		Complete(r)
}
