package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// WriterOptions configures the writer
type WriterOptions struct {
	OutputFile string
	OutputDir  string
}

// Writer writes exported resources to output
type Writer struct {
	opts WriterOptions
}

// NewWriter creates a new writer
func NewWriter(opts WriterOptions) *Writer {
	return &Writer{opts: opts}
}

// Write writes resources to the configured output
func (w *Writer) Write(resources []ExportedResource) error {
	if w.opts.OutputDir != "" {
		return w.writeToDirectory(resources)
	}

	if w.opts.OutputFile != "" {
		return w.writeToFile(resources)
	}

	return w.writeToStdout(resources)
}

func (w *Writer) writeToStdout(resources []ExportedResource) error {
	for i, res := range resources {
		data, err := yaml.Marshal(res.Object)
		if err != nil {
			return fmt.Errorf("failed to marshal %s/%s: %w", res.Kind, res.Name, err)
		}

		// Print document separator before each resource (except first)
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Print(string(data))
	}

	return nil
}

func (w *Writer) writeToFile(resources []ExportedResource) error {
	f, err := os.Create(w.opts.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	for i, res := range resources {
		data, err := yaml.Marshal(res.Object)
		if err != nil {
			return fmt.Errorf("failed to marshal %s/%s: %w", res.Kind, res.Name, err)
		}

		// Write document separator before each resource (except first)
		if i > 0 {
			if _, err := f.WriteString("---\n"); err != nil {
				return err
			}
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) writeToDirectory(resources []ExportedResource) error {
	// Create base directory
	if err := os.MkdirAll(w.opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Group resources by kind
	byKind := make(map[string][]ExportedResource)
	for _, res := range resources {
		byKind[res.Kind] = append(byKind[res.Kind], res)
	}

	// Write each kind to its own subdirectory
	for kind, kindResources := range byKind {
		dirName := kindToDirectory(kind)
		dir := filepath.Join(w.opts.OutputDir, dirName)

		// For single resources (like realm), write directly
		if len(kindResources) == 1 && kind == "KeycloakRealm" {
			res := kindResources[0]
			filename := filepath.Join(w.opts.OutputDir, "realm.yaml")
			if err := w.writeResourceToFile(res, filename); err != nil {
				return err
			}
			continue
		}

		// Create subdirectory for multiple resources
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		for _, res := range kindResources {
			filename := filepath.Join(dir, res.Name+".yaml")
			if err := w.writeResourceToFile(res, filename); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) writeResourceToFile(res ExportedResource, filename string) error {
	data, err := yaml.Marshal(res.Object)
	if err != nil {
		return fmt.Errorf("failed to marshal %s/%s: %w", res.Kind, res.Name, err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	return nil
}

// kindToDirectory maps CRD kinds to directory names
func kindToDirectory(kind string) string {
	// Remove "Keycloak" prefix and convert to kebab-case
	name := strings.TrimPrefix(kind, "Keycloak")

	// Handle special cases
	switch name {
	case "ClientScope":
		return "client-scopes"
	case "IdentityProvider":
		return "identity-providers"
	case "ProtocolMapper":
		return "protocol-mappers"
	case "RoleMapping":
		return "role-mappings"
	}

	// Default: convert to plural lowercase
	return strings.ToLower(name) + "s"
}
