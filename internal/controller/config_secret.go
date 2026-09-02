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

// findForConfigSecret lists objects of the given kind in the Secret's namespace
// and enqueues those whose spec.configSecretRef.name matches the Secret.
func findForConfigSecret(ctx context.Context, c client.Client, secret *corev1.Secret, list client.ObjectList, getRef func(client.Object) *keycloakv1beta1.ConfigSecretRef) []reconcile.Request {
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
		if !ok {
			continue
		}
		ref := getRef(obj)
		if ref != nil && ref.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				},
			})
		}
	}
	return requests
}

// resolveConfigSecretMappings reads the specific keys referenced by each mapping
// from their respective Secrets. It returns a map of configKey → value.
func resolveConfigSecretMappings(ctx context.Context, c client.Client, namespace string, mappings []keycloakv1beta1.ConfigSecretRefMapping) (map[string]string, error) {
	result := make(map[string]string, len(mappings))
	for i, m := range mappings {
		if _, exists := result[m.ConfigKey]; exists {
			return nil, fmt.Errorf("mapping[%d]: duplicate configKey %q; each config key must be set by at most one mapping", i, m.ConfigKey)
		}
		secret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{Name: m.SecretName, Namespace: namespace}, secret); err != nil {
			return nil, fmt.Errorf("mapping[%d]: failed to get secret %q: %w", i, m.SecretName, err)
		}
		val, ok := secret.Data[m.Key]
		if !ok {
			return nil, fmt.Errorf("mapping[%d]: secret %q does not contain key %q", i, m.SecretName, m.Key)
		}
		result[m.ConfigKey] = string(val)
	}
	return result, nil
}

// applyConfigSecretMappings resolves configSecretRefs and merges the mapped
// values into definition.config as single-element string lists. It rejects
// definitions where a config key is set both inline and via a mapping.
func applyConfigSecretMappings(ctx context.Context, c client.Client, namespace string, mappings []keycloakv1beta1.ConfigSecretRefMapping, definition json.RawMessage) (json.RawMessage, error) {
	if len(mappings) == 0 {
		return definition, nil
	}
	data, err := resolveConfigSecretMappings(ctx, c, namespace, mappings)
	if err != nil {
		return nil, err
	}

	// Check for conflicts with inline config keys.
	var defMap map[string]interface{}
	if err := json.Unmarshal(definition, &defMap); err != nil {
		return nil, fmt.Errorf("failed to parse definition: %w", err)
	}
	cfg, _ := defMap["config"].(map[string]interface{})
	for configKey := range data {
		if cfg != nil {
			if _, exists := cfg[configKey]; exists {
				return nil, fmt.Errorf("config key %q is set both inline in definition.config and via configSecretRefs; use one or the other", configKey)
			}
		}
	}

	return mergeDefinitionConfig(definition, data, true), nil
}

// findForConfigSecretRefs lists objects of the given kind in the Secret's
// namespace and enqueues those whose spec.configSecretRefs entries reference
// the Secret by name.
func findForConfigSecretRefs(ctx context.Context, c client.Client, secret *corev1.Secret, list client.ObjectList, getMappings func(client.Object) []keycloakv1beta1.ConfigSecretRefMapping) []reconcile.Request {
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
		if !ok {
			continue
		}
		for _, m := range getMappings(obj) {
			if m.SecretName == secret.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.GetName(),
						Namespace: obj.GetNamespace(),
					},
				})
				break
			}
		}
	}
	return requests
}
