package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

const ReadyConditionType = "Ready"

// setReadyCondition adds or updates the "Ready" condition in the supplied slice.
func setReadyCondition(conditions []metav1.Condition, ready bool, reason, message string) []metav1.Condition {
	condition := metav1.Condition{
		Type:               ReadyConditionType,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	if ready {
		condition.Status = metav1.ConditionTrue
	}

	for i, c := range conditions {
		if c.Type == ReadyConditionType {
			conditions[i] = condition
			return conditions
		}
	}
	return append(conditions, condition)
}

// Default timing constants
const (
	// DefaultSyncPeriod is the default interval for re-checking successfully reconciled resources.
	// This allows detecting drift in Keycloak and ensuring resources stay in sync.
	DefaultSyncPeriod = 5 * time.Minute
)

// Global controller configuration (set once at startup)
var (
	globalSyncPeriod     = DefaultSyncPeriod
	globalSyncPeriodOnce sync.Once
)

// SetSyncPeriod sets the global sync period for all controllers.
// This should only be called once during initialization, before any controllers start.
func SetSyncPeriod(d time.Duration) {
	globalSyncPeriodOnce.Do(func() {
		globalSyncPeriod = d
	})
}

// GetSyncPeriod returns the configured sync period for controllers.
func GetSyncPeriod() time.Duration {
	return globalSyncPeriod
}

// GetKeycloakConfigFromInstance builds the Keycloak client configuration from a KeycloakInstance
func GetKeycloakConfigFromInstance(ctx context.Context, c client.Client, instance *keycloakv1beta1.KeycloakInstance) (keycloak.Config, error) {
	cfg := keycloak.Config{
		BaseURL: instance.Spec.BaseUrl,
	}

	if instance.Spec.Realm != nil {
		cfg.Realm = *instance.Spec.Realm
	}

	auth := instance.Spec.Auth
	switch {
	case auth.ClientCredentials != nil:
		clientID, clientSecret, err := resolveClientCredentials(ctx, c, auth.ClientCredentials, instance.Namespace)
		if err != nil {
			return cfg, err
		}
		cfg.ClientID = clientID
		cfg.ClientSecret = clientSecret
	case auth.PasswordGrant != nil:
		username, password, err := resolvePasswordGrant(ctx, c, auth.PasswordGrant, instance.Namespace)
		if err != nil {
			return cfg, err
		}
		cfg.Username = username
		cfg.Password = password
	default:
		return cfg, fmt.Errorf("auth.passwordGrant or auth.clientCredentials must be set")
	}

	if instance.Spec.TLS != nil {
		cfg.InsecureSkipVerify = instance.Spec.TLS.InsecureSkipVerify
		if instance.Spec.TLS.CACert != nil {
			pem, err := resolveCACert(ctx, c, instance.Spec.TLS.CACert, instance.Namespace)
			if err != nil {
				return cfg, err
			}
			cfg.CACert = pem
		}
	}

	return cfg, nil
}

// GetKeycloakConfigFromClusterInstance builds the Keycloak client configuration from a ClusterKeycloakInstance
func GetKeycloakConfigFromClusterInstance(ctx context.Context, c client.Client, instance *keycloakv1beta1.ClusterKeycloakInstance) (keycloak.Config, error) {
	cfg := keycloak.Config{
		BaseURL: instance.Spec.BaseUrl,
	}

	if instance.Spec.Realm != nil {
		cfg.Realm = *instance.Spec.Realm
	}

	auth := instance.Spec.Auth
	switch {
	case auth.ClientCredentials != nil:
		clientID, clientSecret, err := resolveClusterClientCredentials(ctx, c, auth.ClientCredentials)
		if err != nil {
			return cfg, err
		}
		cfg.ClientID = clientID
		cfg.ClientSecret = clientSecret
	case auth.PasswordGrant != nil:
		username, password, err := resolveClusterPasswordGrant(ctx, c, auth.PasswordGrant)
		if err != nil {
			return cfg, err
		}
		cfg.Username = username
		cfg.Password = password
	default:
		return cfg, fmt.Errorf("auth.passwordGrant or auth.clientCredentials must be set")
	}

	if instance.Spec.TLS != nil {
		cfg.InsecureSkipVerify = instance.Spec.TLS.InsecureSkipVerify
		if instance.Spec.TLS.CACert != nil {
			pem, err := resolveClusterCACert(ctx, c, instance.Spec.TLS.CACert)
			if err != nil {
				return cfg, err
			}
			cfg.CACert = pem
		}
	}

	return cfg, nil
}

// resolvePasswordGrant loads the admin credentials Secret referenced by spec
// and returns (username, password). The inline Username takes precedence over
// the value stored under SecretRef.UsernameKey.
func resolvePasswordGrant(ctx context.Context, c client.Client, spec *keycloakv1beta1.PasswordGrantSpec, defaultNamespace string) (string, string, error) {
	namespace := defaultNamespace
	if spec.SecretRef.Namespace != nil {
		namespace = *spec.SecretRef.Namespace
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: spec.SecretRef.Name, Namespace: namespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get credentials secret: %w", err)
	}

	usernameKey := spec.SecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := spec.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	var username string
	if spec.Username != nil && *spec.Username != "" {
		username = *spec.Username
	} else if v, ok := secret.Data[usernameKey]; ok {
		username = string(v)
	} else {
		return "", "", fmt.Errorf("username key %q not found in secret", usernameKey)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("password key %q not found in secret", passwordKey)
	}
	return username, string(password), nil
}

// resolveClientCredentials loads the client-credentials Secret referenced by
// spec and returns (clientID, clientSecret). The inline ClientID takes
// precedence over the value stored under SecretRef.ClientIdKey.
func resolveClientCredentials(ctx context.Context, c client.Client, spec *keycloakv1beta1.ClientCredentialsSpec, defaultNamespace string) (string, string, error) {
	namespace := defaultNamespace
	if spec.SecretRef.Namespace != nil {
		namespace = *spec.SecretRef.Namespace
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: spec.SecretRef.Name, Namespace: namespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get client credentials secret: %w", err)
	}

	clientIdKey := spec.SecretRef.ClientIdKey
	if clientIdKey == "" {
		clientIdKey = "client-id"
	}
	clientSecretKey := spec.SecretRef.ClientSecretKey
	if clientSecretKey == "" {
		clientSecretKey = "client-secret"
	}

	var clientID string
	if spec.ClientID != nil && *spec.ClientID != "" {
		clientID = *spec.ClientID
	} else if v, ok := secret.Data[clientIdKey]; ok {
		clientID = string(v)
	} else {
		return "", "", fmt.Errorf("client id key %q not found in secret", clientIdKey)
	}

	clientSecret, ok := secret.Data[clientSecretKey]
	if !ok {
		return "", "", fmt.Errorf("client secret key %q not found in secret", clientSecretKey)
	}
	return clientID, string(clientSecret), nil
}

// resolveClusterPasswordGrant is the cluster-scoped variant of
// resolvePasswordGrant; namespace is required on the SecretRef.
func resolveClusterPasswordGrant(ctx context.Context, c client.Client, spec *keycloakv1beta1.ClusterPasswordGrantSpec) (string, string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: spec.SecretRef.Name, Namespace: spec.SecretRef.Namespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get credentials secret: %w", err)
	}

	usernameKey := spec.SecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := spec.SecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	var username string
	if spec.Username != nil && *spec.Username != "" {
		username = *spec.Username
	} else if v, ok := secret.Data[usernameKey]; ok {
		username = string(v)
	} else {
		return "", "", fmt.Errorf("username key %q not found in secret", usernameKey)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("password key %q not found in secret", passwordKey)
	}
	return username, string(password), nil
}

// resolveCACert loads a PEM-encoded CA bundle from the referenced Secret or
// ConfigMap. The CEL XValidation on CACertSource guarantees exactly one of
// secretRef / configMapRef is set.
func resolveCACert(ctx context.Context, c client.Client, src *keycloakv1beta1.CACertSource, defaultNamespace string) (string, error) {
	switch {
	case src.SecretRef != nil:
		namespace := defaultNamespace
		if src.SecretRef.Namespace != nil {
			namespace = *src.SecretRef.Namespace
		}
		key := src.SecretRef.Key
		if key == "" {
			key = "ca.crt"
		}
		secret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{Name: src.SecretRef.Name, Namespace: namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to get caCert secret: %w", err)
		}
		data, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("caCert key %q not found in secret %s/%s", key, namespace, src.SecretRef.Name)
		}
		return string(data), nil
	case src.ConfigMapRef != nil:
		namespace := defaultNamespace
		if src.ConfigMapRef.Namespace != nil {
			namespace = *src.ConfigMapRef.Namespace
		}
		key := src.ConfigMapRef.Key
		if key == "" {
			key = "ca.crt"
		}
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, types.NamespacedName{Name: src.ConfigMapRef.Name, Namespace: namespace}, cm); err != nil {
			return "", fmt.Errorf("failed to get caCert configmap: %w", err)
		}
		data, ok := cm.Data[key]
		if !ok {
			return "", fmt.Errorf("caCert key %q not found in configmap %s/%s", key, namespace, src.ConfigMapRef.Name)
		}
		return data, nil
	default:
		return "", fmt.Errorf("caCert source must set secretRef or configMapRef")
	}
}

// resolveClusterCACert is the cluster-scoped variant of resolveCACert.
func resolveClusterCACert(ctx context.Context, c client.Client, src *keycloakv1beta1.ClusterCACertSource) (string, error) {
	switch {
	case src.SecretRef != nil:
		key := src.SecretRef.Key
		if key == "" {
			key = "ca.crt"
		}
		secret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{Name: src.SecretRef.Name, Namespace: src.SecretRef.Namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to get caCert secret: %w", err)
		}
		data, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("caCert key %q not found in secret %s/%s", key, src.SecretRef.Namespace, src.SecretRef.Name)
		}
		return string(data), nil
	case src.ConfigMapRef != nil:
		key := src.ConfigMapRef.Key
		if key == "" {
			key = "ca.crt"
		}
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, types.NamespacedName{Name: src.ConfigMapRef.Name, Namespace: src.ConfigMapRef.Namespace}, cm); err != nil {
			return "", fmt.Errorf("failed to get caCert configmap: %w", err)
		}
		data, ok := cm.Data[key]
		if !ok {
			return "", fmt.Errorf("caCert key %q not found in configmap %s/%s", key, src.ConfigMapRef.Namespace, src.ConfigMapRef.Name)
		}
		return data, nil
	default:
		return "", fmt.Errorf("caCert source must set secretRef or configMapRef")
	}
}

// resolveClusterClientCredentials is the cluster-scoped variant of
// resolveClientCredentials; namespace is required on the SecretRef.
func resolveClusterClientCredentials(ctx context.Context, c client.Client, spec *keycloakv1beta1.ClusterClientCredentialsSpec) (string, string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: spec.SecretRef.Name, Namespace: spec.SecretRef.Namespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get client credentials secret: %w", err)
	}

	clientIdKey := spec.SecretRef.ClientIdKey
	if clientIdKey == "" {
		clientIdKey = "client-id"
	}
	clientSecretKey := spec.SecretRef.ClientSecretKey
	if clientSecretKey == "" {
		clientSecretKey = "client-secret"
	}

	var clientID string
	if spec.ClientID != nil && *spec.ClientID != "" {
		clientID = *spec.ClientID
	} else if v, ok := secret.Data[clientIdKey]; ok {
		clientID = string(v)
	} else {
		return "", "", fmt.Errorf("client id key %q not found in secret", clientIdKey)
	}

	clientSecret, ok := secret.Data[clientSecretKey]
	if !ok {
		return "", "", fmt.Errorf("client secret key %q not found in secret", clientSecretKey)
	}
	return clientID, string(clientSecret), nil
}

// mergeIDIntoDefinition merges an ID field into a JSON definition
func mergeIDIntoDefinition(definition json.RawMessage, id *string) json.RawMessage {
	if id == nil || *id == "" {
		return definition
	}

	// Parse the definition as a map
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		// If we can't parse, return original
		return definition
	}

	// Add or update the id field
	defMap["id"] = *id

	// Marshal back to JSON
	result, err := json.Marshal(defMap)
	if err != nil {
		return definition
	}

	return result
}

// ptrString is a helper to create a pointer to a string
func ptrString(s string) *string {
	return &s
}

// mergeSmtpCredentials injects SMTP user/password into the definition's smtpServer map.
// If smtpServer doesn't exist yet, it is created.
func mergeSmtpCredentials(definition json.RawMessage, user, password string) json.RawMessage {
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return definition
	}

	smtp, ok := defMap["smtpServer"].(map[string]interface{})
	if !ok {
		smtp = make(map[string]interface{})
	}
	smtp["user"] = user
	smtp["password"] = password
	defMap["smtpServer"] = smtp

	result, err := json.Marshal(defMap)
	if err != nil {
		return definition
	}
	return result
}

// GetKeycloakClientFromRealmInstance resolves the Keycloak API client for a
// KeycloakRealm by following its instanceRef or clusterInstanceRef. This is the
// single source of truth for realm→instance resolution in all child-resource
// controllers (client, user, group, role, etc.). It also returns the Keycloak
// server version reported by the resolved instance, which callers can use for
// version-gated features (organizations, etc.).
func GetKeycloakClientFromRealmInstance(ctx context.Context, c client.Client, clientManager *keycloak.ClientManager, realm *keycloakv1beta1.KeycloakRealm) (*keycloak.Client, string, error) {
	if realm.Spec.ClusterInstanceRef != nil {
		instance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := c.Get(ctx, types.NamespacedName{Name: realm.Spec.ClusterInstanceRef.Name}, instance); err != nil {
			return nil, "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", realm.Spec.ClusterInstanceRef.Name, err)
		}
		if !instance.Status.Ready {
			return nil, "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", realm.Spec.ClusterInstanceRef.Name)
		}
		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, c, instance)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", realm.Spec.ClusterInstanceRef.Name, err)
		}
		kc := clientManager.GetOrCreateClient(clusterInstanceKey(realm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", realm.Spec.ClusterInstanceRef.Name)
		}
		return kc, instance.Status.Version, nil
	}

	if realm.Spec.InstanceRef != nil {
		instanceName := types.NamespacedName{
			Name:      realm.Spec.InstanceRef.Name,
			Namespace: realm.Namespace,
		}
		instance := &keycloakv1beta1.KeycloakInstance{}
		if err := c.Get(ctx, instanceName, instance); err != nil {
			return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
		}
		if !instance.Status.Ready {
			return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
		}
		cfg, err := GetKeycloakConfigFromInstance(ctx, c, instance)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
		}
		kc := clientManager.GetOrCreateClient(instanceName.String(), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
		}
		return kc, instance.Status.Version, nil
	}

	return nil, "", fmt.Errorf("realm %s/%s has neither instanceRef nor clusterInstanceRef", realm.Namespace, realm.Name)
}

// GetKeycloakClientFromClusterRealm resolves the Keycloak admin client and the
// realm name for a ClusterKeycloakRealm referenced by name. It follows the
// cluster realm's instanceRef or clusterInstanceRef. This is the shared
// helper used by all child-resource controllers that need to address a
// resource living under a cluster-scoped realm.
func GetKeycloakClientFromClusterRealm(ctx context.Context, c client.Client, clientManager *keycloak.ClientManager, clusterRealmName string) (*keycloak.Client, string, error) {
	clusterRealm := &keycloakv1beta1.ClusterKeycloakRealm{}
	if err := c.Get(ctx, types.NamespacedName{Name: clusterRealmName}, clusterRealm); err != nil {
		return nil, "", fmt.Errorf("failed to get ClusterKeycloakRealm %s: %w", clusterRealmName, err)
	}

	if !clusterRealm.Status.Ready {
		return nil, "", fmt.Errorf("ClusterKeycloakRealm %s is not ready", clusterRealmName)
	}

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

	if clusterRealm.Spec.ClusterInstanceRef != nil {
		clusterInstance := &keycloakv1beta1.ClusterKeycloakInstance{}
		if err := c.Get(ctx, types.NamespacedName{Name: clusterRealm.Spec.ClusterInstanceRef.Name}, clusterInstance); err != nil {
			return nil, "", fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		if !clusterInstance.Status.Ready {
			return nil, "", fmt.Errorf("ClusterKeycloakInstance %s is not ready", clusterRealm.Spec.ClusterInstanceRef.Name)
		}

		cfg, err := GetKeycloakConfigFromClusterInstance(ctx, c, clusterInstance)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Keycloak config from ClusterKeycloakInstance %s: %w", clusterRealm.Spec.ClusterInstanceRef.Name, err)
		}

		kc := clientManager.GetOrCreateClient(clusterInstanceKey(clusterRealm.Spec.ClusterInstanceRef.Name), cfg)
		if kc == nil {
			return nil, "", fmt.Errorf("Keycloak client not available for cluster instance %s", clusterRealm.Spec.ClusterInstanceRef.Name)
		}
		return kc, realmName, nil
	}

	if clusterRealm.Spec.InstanceRef == nil {
		return nil, "", fmt.Errorf("cluster realm %s has no instanceRef or clusterInstanceRef", clusterRealmName)
	}

	instanceName := types.NamespacedName{
		Name:      clusterRealm.Spec.InstanceRef.Name,
		Namespace: clusterRealm.Spec.InstanceRef.Namespace,
	}

	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := c.Get(ctx, instanceName, instance); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakInstance %s: %w", instanceName, err)
	}

	if !instance.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakInstance %s is not ready", instanceName)
	}

	cfg, err := GetKeycloakConfigFromInstance(ctx, c, instance)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Keycloak config from KeycloakInstance %s: %w", instanceName, err)
	}

	kc := clientManager.GetOrCreateClient(instanceName.String(), cfg)
	if kc == nil {
		return nil, "", fmt.Errorf("Keycloak client not available for instance %s", instanceName)
	}

	return kc, realmName, nil
}

// GetKeycloakClientAndRealmForIDP resolves the Keycloak admin client and the
// realm name for a KeycloakIdentityProvider, following its realmRef or
// clusterRealmRef. This is the shared resolver used by both the
// KeycloakIdentityProvider and KeycloakIdentityProviderMapper controllers.
func GetKeycloakClientAndRealmForIDP(ctx context.Context, c client.Client, clientManager *keycloak.ClientManager, idp *keycloakv1beta1.KeycloakIdentityProvider) (*keycloak.Client, string, error) {
	if idp.Spec.ClusterRealmRef != nil {
		return GetKeycloakClientFromClusterRealm(ctx, c, clientManager, idp.Spec.ClusterRealmRef.Name)
	}

	if idp.Spec.RealmRef == nil {
		return nil, "", fmt.Errorf("either realmRef or clusterRealmRef must be specified")
	}

	realmName := types.NamespacedName{
		Name:      idp.Spec.RealmRef.Name,
		Namespace: idp.Namespace,
	}

	realm := &keycloakv1beta1.KeycloakRealm{}
	if err := c.Get(ctx, realmName, realm); err != nil {
		return nil, "", fmt.Errorf("failed to get KeycloakRealm %s: %w", realmName, err)
	}

	if !realm.Status.Ready {
		return nil, "", fmt.Errorf("KeycloakRealm %s is not ready", realmName)
	}

	var realmDef struct {
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(realm.Spec.Definition.Raw, &realmDef); err != nil {
		return nil, "", fmt.Errorf("failed to parse realm definition: %w", err)
	}

	kc, _, err := GetKeycloakClientFromRealmInstance(ctx, c, clientManager, realm)
	if err != nil {
		return nil, "", err
	}

	return kc, realmDef.Realm, nil
}

// mergeIDPConfig merges the given key-value pairs into definition.config.
// If the config map doesn't exist yet, it is created. Values in secretData
// take precedence over existing entries in definition.config.
func mergeIDPConfig(definition json.RawMessage, secretData map[string]string) json.RawMessage {
	if len(secretData) == 0 {
		return definition
	}

	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return definition
	}

	cfg, ok := defMap["config"].(map[string]interface{})
	if !ok {
		cfg = make(map[string]interface{})
	}
	for k, v := range secretData {
		cfg[k] = v
	}
	defMap["config"] = cfg

	result, err := json.Marshal(defMap)
	if err != nil {
		return definition
	}
	return result
}

// idpDefinitionsMatch compares two IdentityProviderRepresentations for drift-detection,
// stripping fields that Keycloak masks on GET so they don't cause false-positive drift
// (which would otherwise produce an endless update-loop with each reconcile).
//
// Keycloak's `GET /admin/realms/.../identity-provider/instances/{alias}` returns
// `config.clientSecret` as the literal string "**********" instead of the stored value
// — but ONLY when a secret is set. If the IdP has no secret, the field is null/absent.
//
// We strip clientSecret from BOTH sides ONLY when current shows the mask: that means
// Keycloak has *some* secret stored, we can't compare it against our desired value
// anyway, so treat them as equal on that field. If current is null/missing, the IdP
// genuinely has no secret yet and our PUT must push it — leave the field intact so
// the diff fires.
//
// Pace patch: paired with the new drift-detection in keycloakidentityprovider_controller.go.
func idpDefinitionsMatch(desired, current json.RawMessage) bool {
	var desiredMap, currentMap map[string]interface{}
	if err := json.Unmarshal(desired, &desiredMap); err != nil {
		return false
	}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return false
	}

	// If Keycloak masked clientSecret in the current state, drop it from both
	// sides so the unobservable value doesn't cause false drift.
	if cCfg, ok := currentMap["config"].(map[string]interface{}); ok {
		if cs, ok := cCfg["clientSecret"].(string); ok && cs == "**********" {
			delete(cCfg, "clientSecret")
			if dCfg, ok := desiredMap["config"].(map[string]interface{}); ok {
				delete(dCfg, "clientSecret")
			}
		}
	}

	desiredJSON, err := json.Marshal(desiredMap)
	if err != nil {
		return false
	}
	currentJSON, err := json.Marshal(currentMap)
	if err != nil {
		return false
	}
	return definitionsMatch(desiredJSON, currentJSON)
}

// realmDefinitionsMatch compares two RealmRepresentations for drift-detection,
// stripping fields that Keycloak masks on GET so they don't cause false-positive drift.
//
// Keycloak masks `smtpServer.password` as "**********" on GET when an SMTP password
// is configured. If the operator merges a real password from smtpSecretRef into the
// desired definition, a naive comparison would always report drift and force an
// UpdateRealm every reconcile.
//
// We strip smtpServer.password from BOTH sides ONLY when current shows the mask:
// Keycloak has *some* password stored, we can't observe its value, so treat as equal
// on that field. If current is null/missing, the realm genuinely has no password
// stored and the PUT must push it — leave the field intact so the diff fires.
func realmDefinitionsMatch(desired, current json.RawMessage) bool {
	var desiredMap, currentMap map[string]interface{}
	if err := json.Unmarshal(desired, &desiredMap); err != nil {
		return false
	}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return false
	}

	if cSmtp, ok := currentMap["smtpServer"].(map[string]interface{}); ok {
		if pw, ok := cSmtp["password"].(string); ok && pw == "**********" {
			delete(cSmtp, "password")
			if dSmtp, ok := desiredMap["smtpServer"].(map[string]interface{}); ok {
				delete(dSmtp, "password")
			}
		}
	}

	desiredJSON, err := json.Marshal(desiredMap)
	if err != nil {
		return false
	}
	currentJSON, err := json.Marshal(currentMap)
	if err != nil {
		return false
	}
	return definitionsMatch(desiredJSON, currentJSON)
}

// roleCompositesSpec mirrors the "composites" field of a Keycloak
// RoleRepresentation: realm roles by name and client roles keyed by clientId.
type roleCompositesSpec struct {
	Realm  []string            `json:"realm,omitempty"`
	Client map[string][]string `json:"client,omitempty"`
}

// extractRoleComposites returns the desired composite members and whether the
// definition opted into composite handling (via composite: true or a non-empty
// composites object).
func extractRoleComposites(definition json.RawMessage) (composites roleCompositesSpec, requested bool) {
	if len(definition) == 0 {
		return roleCompositesSpec{}, false
	}
	var raw struct {
		Composite  *bool               `json:"composite,omitempty"`
		Composites *roleCompositesSpec `json:"composites,omitempty"`
	}
	if err := json.Unmarshal(definition, &raw); err != nil {
		return roleCompositesSpec{}, false
	}
	if raw.Composites != nil {
		composites = *raw.Composites
	}
	hasMembers := len(composites.Realm) > 0 || len(composites.Client) > 0
	requested = hasMembers || (raw.Composite != nil && *raw.Composite)
	return composites, requested
}

// removeFieldFromDefinition removes a field from a JSON definition.
// If the field doesn't exist or the JSON is invalid, the original is returned unchanged.
func removeFieldFromDefinition(definition json.RawMessage, field string) json.RawMessage {
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return definition
	}

	if _, ok := defMap[field]; !ok {
		return definition
	}

	delete(defMap, field)

	result, err := json.Marshal(defMap)
	if err != nil {
		return definition
	}
	return result
}

// setFieldInDefinition sets a field value in a JSON definition
func setFieldInDefinition(definition json.RawMessage, field string, value interface{}) json.RawMessage {
	// Parse the definition as a map
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		defMap = make(map[string]interface{})
	}

	// Set the field
	defMap[field] = value

	// Marshal back to JSON
	result, err := json.Marshal(defMap)
	if err != nil {
		return definition
	}

	return result
}

var realmFlowBindingFields = []string{
	"browserFlow",
	"registrationFlow",
	"directGrantFlow",
	"resetCredentialsFlow",
	"clientAuthenticationFlow",
	"dockerAuthenticationFlow",
	"firstBrokerLoginFlow",
}

// stripRealmFlowBindingsForCreate removes top-level realm flow bindings from an
// initial realm import. Custom flows managed by KeycloakAuthenticationFlow do
// not exist at realm creation time, so the bindings are applied on a later
// update once the realm is available.
func stripRealmFlowBindingsForCreate(definition json.RawMessage) (json.RawMessage, bool) {
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return definition, false
	}

	changed := false
	for _, field := range realmFlowBindingFields {
		if _, ok := defMap[field]; ok {
			delete(defMap, field)
			changed = true
		}
	}
	if !changed {
		return definition, false
	}

	result, err := json.Marshal(defMap)
	if err != nil {
		return definition, false
	}
	return result, true
}

// flowAliasToKey maps alias-based keys in authenticationFlowBindingOverrides
// to their corresponding Keycloak API keys.
var flowAliasToKey = map[string]string{
	"browserFlowAlias":     "browser",
	"directGrantFlowAlias": "direct_grant",
}

// flowLookupFunc resolves a flow alias to its UUID.
type flowLookupFunc func(ctx context.Context, realmName, alias string) (string, error)

// resolveFlowBindingAliases inspects the definition JSON for
// authenticationFlowBindingOverrides entries that use alias-based keys
// (e.g. browserFlowAlias, directGrantFlowAlias) and resolves them to
// the corresponding Keycloak flow UUIDs via the Admin API.
// UUID-based keys are left untouched. If both an alias key and the
// corresponding UUID key are present, the alias takes precedence.
func resolveFlowBindingAliases(ctx context.Context, kc *keycloak.Client, realmName string, definition json.RawMessage) (json.RawMessage, error) {
	return resolveFlowBindingAliasesWithLookup(ctx, realmName, definition, func(ctx context.Context, realm, alias string) (string, error) {
		flow, err := kc.GetAuthenticationFlowByAlias(ctx, realm, alias)
		if err != nil {
			return "", err
		}
		if flow.ID == nil {
			return "", fmt.Errorf("authentication flow %q has no ID", alias)
		}
		return *flow.ID, nil
	})
}

// resolveFlowBindingAliasesWithLookup is the testable core of alias resolution.
func resolveFlowBindingAliasesWithLookup(ctx context.Context, realmName string, definition json.RawMessage, lookup flowLookupFunc) (json.RawMessage, error) {
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return definition, nil
	}

	overridesRaw, ok := defMap["authenticationFlowBindingOverrides"]
	if !ok {
		return definition, nil
	}

	overrides, ok := overridesRaw.(map[string]interface{})
	if !ok {
		return definition, nil
	}

	changed := false
	for aliasKey, targetKey := range flowAliasToKey {
		aliasVal, exists := overrides[aliasKey]
		if !exists {
			continue
		}

		alias, ok := aliasVal.(string)
		if !ok || alias == "" {
			return nil, fmt.Errorf("authenticationFlowBindingOverrides.%s must be a non-empty string", aliasKey)
		}

		flowID, err := lookup(ctx, realmName, alias)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s %q: %w", aliasKey, alias, err)
		}

		overrides[targetKey] = flowID
		delete(overrides, aliasKey)
		changed = true
	}

	if !changed {
		return definition, nil
	}

	defMap["authenticationFlowBindingOverrides"] = overrides
	result, err := json.Marshal(defMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal definition after flow alias resolution: %w", err)
	}
	return result, nil
}
