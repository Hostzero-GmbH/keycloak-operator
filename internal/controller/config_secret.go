package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// resolveConfigSecret reads all keys from a referenced Secret in namespace.
func resolveConfigSecret(ctx context.Context, c client.Client, namespace string, ref *keycloakv1beta1.ConfigSecretRef) (map[string]string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return nil, fmt.Errorf("failed to get config secret %q: %w", ref.Name, err)
	}

	data := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		data[k] = string(v)
	}
	return data, nil
}

// applyConfigSecret merges spec.configSecretRef into definition.config.
// wrapAsList is true for ComponentRepresentation config (map[string][]string).
func applyConfigSecret(ctx context.Context, c client.Client, namespace string, ref *keycloakv1beta1.ConfigSecretRef, definition json.RawMessage, wrapAsList bool) (json.RawMessage, error) {
	if ref == nil {
		return definition, nil
	}
	data, err := resolveConfigSecret(ctx, c, namespace, ref)
	if err != nil {
		return nil, err
	}
	return mergeDefinitionConfig(definition, data, wrapAsList), nil
}

// applyConfigSecretMappings merges spec.configSecretRefs into definition.config
// as single-element string lists. Runs after applyConfigSecret, so a config key
// already present (inline or via configSecretRef) is a conflict rather than an
// override.
func applyConfigSecretMappings(ctx context.Context, c client.Client, namespace string, mappings []keycloakv1beta1.ConfigSecretRefMapping, definition json.RawMessage) (json.RawMessage, error) {
	if len(mappings) == 0 {
		return definition, nil
	}

	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return nil, fmt.Errorf("failed to parse definition: %w", err)
	}
	cfg, _ := defMap["config"].(map[string]interface{})

	data := make(map[string]string, len(mappings))
	for _, m := range mappings {
		if _, exists := cfg[m.ConfigKey]; exists {
			return nil, fmt.Errorf("config key %q is already set in definition.config; it cannot also be set via configSecretRefs", m.ConfigKey)
		}
		secret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{Name: m.SecretName, Namespace: namespace}, secret); err != nil {
			return nil, fmt.Errorf("failed to get secret %q for config key %q: %w", m.SecretName, m.ConfigKey, err)
		}
		val, ok := secret.Data[m.Key]
		if !ok {
			return nil, fmt.Errorf("secret %q does not contain key %q", m.SecretName, m.Key)
		}
		data[m.ConfigKey] = string(val)
	}

	return mergeDefinitionConfig(definition, data, true), nil
}

// findForConfigSecret lists objects of the given kind in the Secret's namespace
// and enqueues those whose spec.configSecretRef.name matches the Secret.
func findForConfigSecret(ctx context.Context, c client.Client, secret *corev1.Secret, list client.ObjectList, getRef func(client.Object) *keycloakv1beta1.ConfigSecretRef) []reconcile.Request {
	return findReferencingSecret(ctx, c, secret, list, func(o client.Object) bool {
		ref := getRef(o)
		return ref != nil && ref.Name == secret.Name
	})
}

// findReferencingSecret lists objects of the given kind in the Secret's
// namespace and enqueues those for which references reports true.
func findReferencingSecret(ctx context.Context, c client.Client, secret *corev1.Secret, list client.ObjectList, references func(client.Object) bool) []reconcile.Request {
	if err := c.List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, item := range items {
		obj, ok := item.(client.Object)
		if !ok || !references(obj) {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
		})
	}
	return requests
}
