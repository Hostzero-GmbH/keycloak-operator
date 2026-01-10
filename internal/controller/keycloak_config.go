package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

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

	if err := c.Get(ctx, secretName, secret); err != nil {
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

// GetKeycloakConfigFromClusterInstance builds the Keycloak client configuration from a ClusterKeycloakInstance
func GetKeycloakConfigFromClusterInstance(ctx context.Context, c client.Client, instance *keycloakv1beta1.ClusterKeycloakInstance) (keycloak.Config, error) {
	cfg := keycloak.Config{
		BaseURL: instance.Spec.BaseUrl,
	}

	if instance.Spec.Realm != nil {
		cfg.Realm = *instance.Spec.Realm
	}

	// Get credentials secret (namespace is required for cluster-scoped resources)
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretRef.Name,
		Namespace: instance.Spec.Credentials.SecretRef.Namespace,
	}

	if err := c.Get(ctx, secretName, secret); err != nil {
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
