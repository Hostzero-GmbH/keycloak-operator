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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
	"github.com/hostzero/keycloak-operator/internal/keycloak"
)

// KeycloakProtocolMapperReconciler reconciles a KeycloakProtocolMapper object
type KeycloakProtocolMapperReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keycloak.hostzero.com,resources=keycloakprotocolmappers/finalizers,verbs=update

// Reconcile handles KeycloakProtocolMapper reconciliation
func (r *KeycloakProtocolMapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	mapper := &keycloakv1beta1.KeycloakProtocolMapper{}
	if err := r.Get(ctx, req.NamespacedName, mapper); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KeycloakProtocolMapper")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !mapper.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mapper, FinalizerName) {
			if err := r.deleteMapper(ctx, mapper); err != nil {
				log.Error(err, "failed to delete mapper from Keycloak")
			}
			controllerutil.RemoveFinalizer(mapper, FinalizerName)
			if err := r.Update(ctx, mapper); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(mapper, FinalizerName) {
		controllerutil.AddFinalizer(mapper, FinalizerName)
		if err := r.Update(ctx, mapper); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Keycloak client and parent info
	kc, realmName, parentID, parentType, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		log.Error(err, "failed to get Keycloak client")
		return r.updateStatus(ctx, mapper, false, "Error", err.Error())
	}

	// Parse mapper definition
	var mapperDef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(mapper.Spec.Definition.Raw, &mapperDef); err != nil {
		log.Error(err, "failed to parse mapper definition")
		return r.updateStatus(ctx, mapper, false, "Error", fmt.Sprintf("Invalid definition: %v", err))
	}

	// Create or update mapper
	if parentType == "client" {
		mapperID, err := kc.CreateClientProtocolMapper(ctx, realmName, parentID, mapper.Spec.Definition.Raw)
		if err != nil {
			// Try update
			if err := kc.UpdateClientProtocolMapper(ctx, realmName, parentID, mapper.Status.MapperID, mapper.Spec.Definition.Raw); err != nil {
				log.Error(err, "failed to update mapper")
				return r.updateStatus(ctx, mapper, false, "Error", fmt.Sprintf("Failed: %v", err))
			}
		} else {
			mapper.Status.MapperID = mapperID
		}
	} else {
		mapperID, err := kc.CreateClientScopeProtocolMapper(ctx, realmName, parentID, mapper.Spec.Definition.Raw)
		if err != nil {
			if err := kc.UpdateClientScopeProtocolMapper(ctx, realmName, parentID, mapper.Status.MapperID, mapper.Spec.Definition.Raw); err != nil {
				log.Error(err, "failed to update mapper")
				return r.updateStatus(ctx, mapper, false, "Error", fmt.Sprintf("Failed: %v", err))
			}
		} else {
			mapper.Status.MapperID = mapperID
		}
	}

	log.Info("mapper synchronized", "name", mapperDef.Name)
	return r.updateStatus(ctx, mapper, true, "Ready", "Mapper synchronized")
}

func (r *KeycloakProtocolMapperReconciler) updateStatus(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper, ready bool, status, message string) (ctrl.Result, error) {
	mapper.Status.Ready = ready
	mapper.Status.Status = status
	mapper.Status.Message = message

	if err := r.Status().Update(ctx, mapper); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *KeycloakProtocolMapperReconciler) getKeycloakClientAndParent(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) (*keycloak.Client, string, string, string, error) {
	var realmName string
	var parentID string
	var parentType string
	var instance *keycloakv1beta1.KeycloakInstance

	if mapper.Spec.ClientRef != nil {
		// Get client
		kcClient := &keycloakv1beta1.KeycloakClient{}
		clientName := types.NamespacedName{
			Name:      mapper.Spec.ClientRef.Name,
			Namespace: mapper.Namespace,
		}
		if mapper.Spec.ClientRef.Namespace != nil {
			clientName.Namespace = *mapper.Spec.ClientRef.Namespace
		}
		if err := r.Get(ctx, clientName, kcClient); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get client: %w", err)
		}
		parentID = kcClient.Status.ClientUUID
		parentType = "client"

		// Get realm from client
		realm := &keycloakv1beta1.KeycloakRealm{}
		realmRef := types.NamespacedName{
			Name:      kcClient.Spec.RealmRef.Name,
			Namespace: kcClient.Namespace,
		}
		if kcClient.Spec.RealmRef.Namespace != nil {
			realmRef.Namespace = *kcClient.Spec.RealmRef.Namespace
		}
		if err := r.Get(ctx, realmRef, realm); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get realm: %w", err)
		}

		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", "", "", err
		}
		realmName = realmDef.Realm

		// Get instance
		instance = &keycloakv1beta1.KeycloakInstance{}
		instanceName := types.NamespacedName{
			Name:      realm.Spec.InstanceRef.Name,
			Namespace: realm.Namespace,
		}
		if realm.Spec.InstanceRef.Namespace != nil {
			instanceName.Namespace = *realm.Spec.InstanceRef.Namespace
		}
		if err := r.Get(ctx, instanceName, instance); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get instance: %w", err)
		}
	} else if mapper.Spec.ClientScopeRef != nil {
		// Get client scope
		scope := &keycloakv1beta1.KeycloakClientScope{}
		scopeName := types.NamespacedName{
			Name:      mapper.Spec.ClientScopeRef.Name,
			Namespace: mapper.Namespace,
		}
		if mapper.Spec.ClientScopeRef.Namespace != nil {
			scopeName.Namespace = *mapper.Spec.ClientScopeRef.Namespace
		}
		if err := r.Get(ctx, scopeName, scope); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get client scope: %w", err)
		}
		parentID = scope.Status.ScopeID
		parentType = "clientScope"

		// Get realm from scope
		realm := &keycloakv1beta1.KeycloakRealm{}
		realmRef := types.NamespacedName{
			Name:      scope.Spec.RealmRef.Name,
			Namespace: scope.Namespace,
		}
		if scope.Spec.RealmRef.Namespace != nil {
			realmRef.Namespace = *scope.Spec.RealmRef.Namespace
		}
		if err := r.Get(ctx, realmRef, realm); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get realm: %w", err)
		}

		var realmDef struct {
			Realm string `json:"realm"`
		}
		if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
			return nil, "", "", "", err
		}
		realmName = realmDef.Realm

		// Get instance
		instance = &keycloakv1beta1.KeycloakInstance{}
		instanceName := types.NamespacedName{
			Name:      realm.Spec.InstanceRef.Name,
			Namespace: realm.Namespace,
		}
		if realm.Spec.InstanceRef.Namespace != nil {
			instanceName.Namespace = *realm.Spec.InstanceRef.Namespace
		}
		if err := r.Get(ctx, instanceName, instance); err != nil {
			return nil, "", "", "", fmt.Errorf("failed to get instance: %w", err)
		}
	} else {
		return nil, "", "", "", fmt.Errorf("either clientRef or clientScopeRef must be specified")
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, r.Client, instance)
	if err != nil {
		return nil, "", "", "", err
	}

	return keycloak.NewClient(cfg, log.FromContext(ctx)), realmName, parentID, parentType, nil
}

func (r *KeycloakProtocolMapperReconciler) deleteMapper(ctx context.Context, mapper *keycloakv1beta1.KeycloakProtocolMapper) error {
	if mapper.Status.MapperID == "" {
		return nil
	}

	kc, realmName, parentID, parentType, err := r.getKeycloakClientAndParent(ctx, mapper)
	if err != nil {
		return err
	}

	if parentType == "client" {
		return kc.DeleteClientProtocolMapper(ctx, realmName, parentID, mapper.Status.MapperID)
	}
	return kc.DeleteClientScopeProtocolMapper(ctx, realmName, parentID, mapper.Status.MapperID)
}

// SetupWithManager sets up the controller with the Manager
func (r *KeycloakProtocolMapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keycloakv1beta1.KeycloakProtocolMapper{}).
		Complete(r)
}
