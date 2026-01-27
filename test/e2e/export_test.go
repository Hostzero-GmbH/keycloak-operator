package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/export"
)

func TestExportBasicRealm(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// Setup: Create a test realm with some resources
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "export")

	// Get Keycloak client
	kc := getInternalKeycloakClient(t)
	log := ctrl.Log.WithName("export-test")

	// Create exporter
	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		SkipDefaults:    true,
	})

	// Run export
	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	// Verify realm was exported
	var foundRealm bool
	for _, res := range resources {
		if res.Kind == "KeycloakRealm" {
			foundRealm = true
			assert.Contains(t, res.Name, "test-realm")
		}
	}
	assert.True(t, foundRealm, "Expected to find KeycloakRealm in exported resources")
}

func TestExportWithFiltering(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// Setup
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "filter")

	kc := getInternalKeycloakClient(t)
	log := ctrl.Log.WithName("export-test")

	// Export only realm (exclude everything else)
	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		Include:         []string{"realm"},
		SkipDefaults:    true,
	})

	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)

	// Should only have the realm
	assert.Len(t, resources, 1)
	assert.Equal(t, "KeycloakRealm", resources[0].Kind)
}

func TestExportSkipsDefaultResources(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// Setup
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "defaults")

	kc := getInternalKeycloakClient(t)
	log := ctrl.Log.WithName("export-test")

	// Export with skip-defaults enabled
	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		SkipDefaults:    true,
	})

	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)

	// Check that default clients are not exported
	for _, res := range resources {
		if res.Kind == "KeycloakClient" {
			// These are default Keycloak clients that should be skipped
			assert.NotEqual(t, "account", res.Name, "Default 'account' client should be skipped")
			assert.NotEqual(t, "account-console", res.Name, "Default 'account-console' client should be skipped")
			assert.NotEqual(t, "admin-cli", res.Name, "Default 'admin-cli' client should be skipped")
			assert.NotEqual(t, "broker", res.Name, "Default 'broker' client should be skipped")
			assert.NotEqual(t, "realm-management", res.Name, "Default 'realm-management' client should be skipped")
			assert.NotEqual(t, "security-admin-console", res.Name, "Default 'security-admin-console' client should be skipped")
		}
	}
}

func TestExportIncludesDefaultsWhenRequested(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// Setup
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "withdefaults")

	kc := getInternalKeycloakClient(t)
	log := ctrl.Log.WithName("export-test")

	// Export with skip-defaults disabled
	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		Include:         []string{"clients"},
		SkipDefaults:    false, // Include defaults
	})

	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)

	// Should have default clients
	var foundAccountClient bool
	for _, res := range resources {
		if res.Kind == "KeycloakClient" && res.Name == "account" {
			foundAccountClient = true
			break
		}
	}
	assert.True(t, foundAccountClient, "Expected to find default 'account' client when skip-defaults is disabled")
}

func TestExportWriterStdout(t *testing.T) {
	// Test that writer can output to stdout (captured via buffer)
	resources := []export.ExportedResource{
		{
			Kind:       "KeycloakRealm",
			Name:       "test-realm",
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Object: map[string]interface{}{
				"apiVersion": "keycloak.hostzero.com/v1beta1",
				"kind":       "KeycloakRealm",
				"metadata": map[string]interface{}{
					"name":      "test-realm",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"instanceRef": map[string]interface{}{
						"name": "keycloak-instance",
					},
				},
			},
		},
	}

	// Use a temp file instead of stdout for testing
	tmpFile, err := os.CreateTemp("", "export-test-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	writer := export.NewWriter(export.WriterOptions{
		OutputFile: tmpFile.Name(),
	})

	err = writer.Write(resources)
	require.NoError(t, err)

	// Read back and verify
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	assert.Contains(t, string(content), "KeycloakRealm")
	assert.Contains(t, string(content), "test-realm")
}

func TestExportWriterDirectory(t *testing.T) {
	// Test directory output structure
	resources := []export.ExportedResource{
		{
			Kind:       "KeycloakRealm",
			Name:       "test-realm",
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Object: map[string]interface{}{
				"apiVersion": "keycloak.hostzero.com/v1beta1",
				"kind":       "KeycloakRealm",
				"metadata": map[string]interface{}{
					"name":      "test-realm",
					"namespace": "default",
				},
			},
		},
		{
			Kind:       "KeycloakClient",
			Name:       "my-client",
			APIVersion: "keycloak.hostzero.com/v1beta1",
			Object: map[string]interface{}{
				"apiVersion": "keycloak.hostzero.com/v1beta1",
				"kind":       "KeycloakClient",
				"metadata": map[string]interface{}{
					"name":      "my-client",
					"namespace": "default",
				},
			},
		},
	}

	tmpDir, err := os.MkdirTemp("", "export-test-dir")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	writer := export.NewWriter(export.WriterOptions{
		OutputDir: tmpDir,
	})

	err = writer.Write(resources)
	require.NoError(t, err)

	// Verify directory structure
	realmFile := tmpDir + "/realm.yaml"
	clientsDir := tmpDir + "/clients"

	assert.FileExists(t, realmFile)
	assert.DirExists(t, clientsDir)

	clientFile := clientsDir + "/my-client.yaml"
	assert.FileExists(t, clientFile)
}

func TestTransformerSanitizesNames(t *testing.T) {
	transformer := export.NewTransformer(export.TransformerOptions{
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		RealmRef:        "my-realm",
	})

	// Test with a name that needs sanitizing
	rawClient := json.RawMessage(`{"clientId": "My Client With Spaces!", "name": "Test"}`)
	resource, err := transformer.TransformClient(rawClient, "My Client With Spaces!")
	require.NoError(t, err)

	// Name should be sanitized to lowercase with dashes (trailing dashes trimmed)
	assert.Equal(t, "my-client-with-spaces", resource.Name)
	assert.NotContains(t, resource.Name, " ")
	assert.NotContains(t, resource.Name, "!")
}

func TestTransformerRemovesServerManagedFields(t *testing.T) {
	transformer := export.NewTransformer(export.TransformerOptions{
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		RealmRef:        "my-realm",
	})

	// Raw client with server-managed fields
	rawClient := json.RawMessage(`{
		"id": "12345678-1234-1234-1234-123456789012",
		"clientId": "my-client",
		"secret": "super-secret-value",
		"name": "My Client"
	}`)

	resource, err := transformer.TransformClient(rawClient, "my-client")
	require.NoError(t, err)

	// The definition should not contain the id or secret
	objBytes, err := yaml.Marshal(resource.Object)
	require.NoError(t, err)

	assert.NotContains(t, string(objBytes), "12345678-1234-1234-1234-123456789012")
	assert.NotContains(t, string(objBytes), "super-secret-value")
}

func TestFilterIncludeExclude(t *testing.T) {
	// Test include filter
	filter := export.NewFilter([]string{"clients", "users"}, nil, true)
	assert.True(t, filter.ShouldIncludeType("clients"))
	assert.True(t, filter.ShouldIncludeType("users"))
	assert.False(t, filter.ShouldIncludeType("groups"))
	assert.False(t, filter.ShouldIncludeType("roles"))

	// Test exclude filter
	filter2 := export.NewFilter(nil, []string{"roles", "role-mappings"}, true)
	assert.True(t, filter2.ShouldIncludeType("clients"))
	assert.True(t, filter2.ShouldIncludeType("users"))
	assert.False(t, filter2.ShouldIncludeType("roles"))
	assert.False(t, filter2.ShouldIncludeType("role-mappings"))
}

func TestFilterSkipsDefaultClients(t *testing.T) {
	filter := export.NewFilter(nil, nil, true) // skipDefaults = true

	assert.True(t, filter.ShouldSkipClient("account"))
	assert.True(t, filter.ShouldSkipClient("account-console"))
	assert.True(t, filter.ShouldSkipClient("admin-cli"))
	assert.True(t, filter.ShouldSkipClient("broker"))
	assert.True(t, filter.ShouldSkipClient("realm-management"))
	assert.True(t, filter.ShouldSkipClient("security-admin-console"))

	// Custom clients should not be skipped
	assert.False(t, filter.ShouldSkipClient("my-custom-client"))
	assert.False(t, filter.ShouldSkipClient("webapp"))
}

func TestFilterSkipsServiceAccountUsers(t *testing.T) {
	filter := export.NewFilter(nil, nil, true)

	assert.True(t, filter.ShouldSkipUser("service-account-my-client"))
	assert.True(t, filter.ShouldSkipUser("service-account-webapp"))

	// Regular users should not be skipped
	assert.False(t, filter.ShouldSkipUser("admin"))
	assert.False(t, filter.ShouldSkipUser("john.doe"))
}

func TestExportGeneratesValidYAML(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// Setup
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "yaml-valid")

	kc := getInternalKeycloakClient(t)
	log := ctrl.Log.WithName("export-test")

	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "test-namespace",
		InstanceRef:     "my-keycloak",
		RealmRef:        realmName,
		Include:         []string{"realm"},
		SkipDefaults:    true,
	})

	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	// Write to buffer
	var buf bytes.Buffer
	for _, res := range resources {
		yamlBytes, err := yaml.Marshal(res.Object)
		require.NoError(t, err)
		buf.Write(yamlBytes)
		buf.WriteString("---\n")
	}

	// Verify YAML is valid by parsing it back
	output := buf.String()
	docs := strings.Split(output, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var parsed map[string]interface{}
		err := yaml.Unmarshal([]byte(doc), &parsed)
		require.NoError(t, err, "Generated YAML should be valid")

		// Verify required fields
		assert.Contains(t, parsed, "apiVersion")
		assert.Contains(t, parsed, "kind")
		assert.Contains(t, parsed, "metadata")
		assert.Contains(t, parsed, "spec")
	}
}

func TestExportKeycloakClientMethods(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	kc := getInternalKeycloakClient(t)

	// Test GetRealmRoles
	roles, err := kc.GetRealmRoles(context.Background(), "master")
	require.NoError(t, err)
	assert.NotEmpty(t, roles, "Expected at least some realm roles in master realm")

	// Test GetIdentityProviders (may be empty, but should not error)
	_, err = kc.GetIdentityProviders(context.Background(), "master")
	require.NoError(t, err)

	// Test GetRaw
	rawRealm, err := kc.GetRealmRaw(context.Background(), "master")
	require.NoError(t, err)
	assert.NotEmpty(t, rawRealm)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(rawRealm, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "master", parsed["realm"])
}

func TestExportWithCustomClient(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNoKeycloakAccess(t)

	// This test creates a custom client and verifies it's exported correctly
	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "custom-client")

	kc := getInternalKeycloakClient(t)

	// Create a custom client directly in Keycloak
	clientDef := json.RawMessage(`{
		"clientId": "export-test-client",
		"name": "Export Test Client",
		"enabled": true,
		"publicClient": false,
		"standardFlowEnabled": true
	}`)
	_, err := kc.CreateClient(context.Background(), realmName, clientDef)
	require.NoError(t, err)

	// Export the realm
	log := ctrl.Log.WithName("export-test")
	exporter := export.NewExporter(kc, log, export.ExporterOptions{
		Realm:           realmName,
		TargetNamespace: "default",
		InstanceRef:     "keycloak-instance",
		Include:         []string{"clients"},
		SkipDefaults:    true,
	})

	resources, err := exporter.Export(context.Background())
	require.NoError(t, err)

	// Find our custom client
	var foundCustomClient bool
	for _, res := range resources {
		if res.Kind == "KeycloakClient" && res.Name == "export-test-client" {
			foundCustomClient = true
			break
		}
	}
	assert.True(t, foundCustomClient, "Expected to find custom 'export-test-client' in exported resources")
}
