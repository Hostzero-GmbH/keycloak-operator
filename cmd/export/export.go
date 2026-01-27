// Package export provides the CLI for exporting Keycloak resources to Kubernetes CRD manifests.
package export

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Hostzero-GmbH/keycloak-operator/internal/export"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// Run executes the export command with the given arguments
func Run(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	opts := &Options{}
	opts.BindFlags(fs)

	// Parse flags
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	zapOpts := zap.Options{Development: opts.Verbose}
	log := zap.New(zap.UseFlagOptions(&zapOpts))

	// Validate options
	if err := opts.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fs.Usage()
		os.Exit(1)
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, cleaning up...")
		cancel()
	}()

	// Run export
	if err := runExport(ctx, opts, log); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runExport(ctx context.Context, opts *Options, log logr.Logger) error {
	// Get Keycloak configuration
	cfg, err := opts.GetKeycloakConfig(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to get Keycloak configuration: %w", err)
	}

	// Create Keycloak client
	client := keycloak.NewClient(*cfg, log)

	// Test connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Keycloak at %s: %w", cfg.BaseURL, err)
	}

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Connected to Keycloak at %s\n", cfg.BaseURL)
	}

	// Create exporter
	exporter := export.NewExporter(client, log, export.ExporterOptions{
		Realm:           opts.Realm,
		TargetNamespace: opts.TargetNamespace,
		InstanceRef:     opts.InstanceRef,
		RealmRef:        opts.RealmRef,
		Include:         opts.Include,
		Exclude:         opts.Exclude,
		SkipDefaults:    opts.SkipDefaults,
	})

	// Run export
	resources, err := exporter.Export(ctx)
	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Exported %d resources\n", len(resources))
	}

	// Write output
	writer := export.NewWriter(export.WriterOptions{
		OutputFile: opts.Output,
		OutputDir:  opts.OutputDir,
	})

	if err := writer.Write(resources); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
