package export

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

// Options holds the export command options
type Options struct {
	// Connection options (direct mode)
	URL      string
	Username string
	Password string

	// Connection options (from-instance mode)
	FromInstance        string
	FromClusterInstance string
	Namespace           string

	// Export options
	Realm string

	// Output options
	Output    string
	OutputDir string

	// Manifest generation options
	TargetNamespace string
	InstanceRef     string
	RealmRef        string

	// Filtering options
	Include      []string
	Exclude      []string
	SkipDefaults bool

	// General options
	Verbose bool

	// Internal
	includeRaw string
	excludeRaw string
}

// BindFlags binds the options to the given flag set
func (o *Options) BindFlags(fs *flag.FlagSet) {
	// Connection options (direct mode)
	fs.StringVar(&o.URL, "url", "", "Keycloak server URL (e.g., https://keycloak.example.com)")
	fs.StringVar(&o.Username, "username", "", "Keycloak admin username")
	fs.StringVar(&o.Password, "password", "", "Keycloak admin password (use env var KEYCLOAK_PASSWORD for security)")

	// Connection options (from-instance mode)
	fs.StringVar(&o.FromInstance, "from-instance", "", "Name of KeycloakInstance CR to read connection details from")
	fs.StringVar(&o.FromClusterInstance, "from-cluster-instance", "", "Name of ClusterKeycloakInstance CR to read connection details from")
	fs.StringVar(&o.Namespace, "namespace", "", "Namespace of the KeycloakInstance CR (required with --from-instance)")

	// Export options
	fs.StringVar(&o.Realm, "realm", "", "Realm to export (required)")

	// Output options
	fs.StringVar(&o.Output, "output", "", "Output file path (default: stdout)")
	fs.StringVar(&o.OutputDir, "output-dir", "", "Output directory for multiple files (creates directory structure)")

	// Manifest generation options
	fs.StringVar(&o.TargetNamespace, "target-namespace", "default", "Namespace for generated manifests")
	fs.StringVar(&o.InstanceRef, "instance-ref", "", "Name of KeycloakInstance to reference in generated manifests")
	fs.StringVar(&o.RealmRef, "realm-ref", "", "Name of KeycloakRealm to reference (defaults to realm name)")

	// Filtering options
	fs.StringVar(&o.includeRaw, "include", "", "Comma-separated list of resource types to include (e.g., clients,users,groups)")
	fs.StringVar(&o.excludeRaw, "exclude", "", "Comma-separated list of resource types to exclude")
	fs.BoolVar(&o.SkipDefaults, "skip-defaults", true, "Skip Keycloak built-in resources (default clients, roles, etc.)")

	// General options
	fs.BoolVar(&o.Verbose, "verbose", false, "Enable verbose output")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: keycloak-operator export [options]

Export Keycloak resources to Kubernetes CRD manifests.

Connection Options (choose one mode):

  Direct connection:
    --url           Keycloak server URL
    --username      Admin username
    --password      Admin password (or use KEYCLOAK_PASSWORD env var)

  From existing KeycloakInstance CR:
    --from-instance          Name of KeycloakInstance CR
    --from-cluster-instance  Name of ClusterKeycloakInstance CR
    --namespace              Namespace of KeycloakInstance

Export Options:
    --realm         Realm to export (required)

Output Options:
    --output        Output file path (default: stdout)
    --output-dir    Output directory (creates file structure)

Manifest Options:
    --target-namespace  Namespace for generated manifests (default: "default")
    --instance-ref      KeycloakInstance name to reference
    --realm-ref         KeycloakRealm name to reference

Filtering Options:
    --include       Resource types to include (comma-separated)
    --exclude       Resource types to exclude (comma-separated)
    --skip-defaults Skip built-in Keycloak resources (default: true)

Resource types: realm, clients, client-scopes, users, groups, roles, 
                role-mappings, identity-providers, components, 
                protocol-mappers, organizations

Examples:

  # Export realm to stdout
  keycloak-operator export \
    --url https://keycloak.example.com \
    --username admin \
    --password "$KEYCLOAK_PASSWORD" \
    --realm my-realm

  # Export to directory structure
  keycloak-operator export \
    --url https://keycloak.example.com \
    --username admin \
    --password "$KEYCLOAK_PASSWORD" \
    --realm my-realm \
    --output-dir ./manifests

  # Export using existing KeycloakInstance CR
  keycloak-operator export \
    --from-instance my-keycloak \
    --namespace keycloak-operator \
    --realm my-realm

  # Export only clients and users
  keycloak-operator export \
    --url https://keycloak.example.com \
    --username admin \
    --password "$KEYCLOAK_PASSWORD" \
    --realm my-realm \
    --include clients,users

`)
		fs.PrintDefaults()
	}
}

// Validate validates the options
func (o *Options) Validate() error {
	// Parse include/exclude lists
	if o.includeRaw != "" {
		o.Include = strings.Split(o.includeRaw, ",")
		for i := range o.Include {
			o.Include[i] = strings.TrimSpace(o.Include[i])
		}
	}
	if o.excludeRaw != "" {
		o.Exclude = strings.Split(o.excludeRaw, ",")
		for i := range o.Exclude {
			o.Exclude[i] = strings.TrimSpace(o.Exclude[i])
		}
	}

	// Check password from environment if not provided
	if o.Password == "" {
		o.Password = os.Getenv("KEYCLOAK_PASSWORD")
	}

	// Validate connection mode
	directMode := o.URL != ""
	instanceMode := o.FromInstance != "" || o.FromClusterInstance != ""

	if !directMode && !instanceMode {
		return fmt.Errorf("either --url or --from-instance/--from-cluster-instance is required")
	}

	if directMode && instanceMode {
		return fmt.Errorf("cannot use both --url and --from-instance/--from-cluster-instance")
	}

	if directMode {
		if o.Username == "" {
			return fmt.Errorf("--username is required when using --url")
		}
		if o.Password == "" {
			return fmt.Errorf("--password is required when using --url (or set KEYCLOAK_PASSWORD env var)")
		}
	}

	if o.FromInstance != "" && o.Namespace == "" {
		return fmt.Errorf("--namespace is required when using --from-instance")
	}

	// Validate realm
	if o.Realm == "" {
		return fmt.Errorf("--realm is required")
	}

	// Validate output options
	if o.Output != "" && o.OutputDir != "" {
		return fmt.Errorf("cannot use both --output and --output-dir")
	}

	return nil
}

// GetKeycloakConfig returns the Keycloak client configuration
func (o *Options) GetKeycloakConfig(ctx context.Context, log logr.Logger) (*keycloak.Config, error) {
	if o.URL != "" {
		// Direct mode
		return &keycloak.Config{
			BaseURL:  o.URL,
			Realm:    "master",
			Username: o.Username,
			Password: o.Password,
		}, nil
	}

	// From-instance mode - need to read from Kubernetes
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w (ensure KUBECONFIG is set or ~/.kube/config exists)", err)
	}

	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	if o.FromClusterInstance != "" {
		return o.loadFromClusterInstance(ctx, k8sClient, log)
	}

	return o.loadFromInstance(ctx, k8sClient, log)
}

func (o *Options) loadFromInstance(ctx context.Context, k8sClient client.Client, log logr.Logger) (*keycloak.Config, error) {
	// Get KeycloakInstance
	instance := &keycloakv1beta1.KeycloakInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      o.FromInstance,
		Namespace: o.Namespace,
	}, instance); err != nil {
		return nil, fmt.Errorf("failed to get KeycloakInstance %s/%s: %w", o.Namespace, o.FromInstance, err)
	}

	// Get credentials secret
	secretNamespace := o.Namespace
	if instance.Spec.Credentials.SecretRef.Namespace != nil {
		secretNamespace = *instance.Spec.Credentials.SecretRef.Namespace
	}

	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s/%s: %w", secretNamespace, instance.Spec.Credentials.SecretRef.Name, err)
	}

	usernameKey := "username"
	if instance.Spec.Credentials.SecretRef.UsernameKey != "" {
		usernameKey = instance.Spec.Credentials.SecretRef.UsernameKey
	}
	passwordKey := "password"
	if instance.Spec.Credentials.SecretRef.PasswordKey != "" {
		passwordKey = instance.Spec.Credentials.SecretRef.PasswordKey
	}

	username := string(secret.Data[usernameKey])
	password := string(secret.Data[passwordKey])

	if username == "" || password == "" {
		return nil, fmt.Errorf("credentials secret is missing username or password")
	}

	adminRealm := "master"
	if instance.Spec.Realm != nil {
		adminRealm = *instance.Spec.Realm
	}

	return &keycloak.Config{
		BaseURL:  instance.Spec.BaseUrl,
		Realm:    adminRealm,
		Username: username,
		Password: password,
	}, nil
}

func (o *Options) loadFromClusterInstance(ctx context.Context, k8sClient client.Client, log logr.Logger) (*keycloak.Config, error) {
	// Get ClusterKeycloakInstance
	instance := &keycloakv1beta1.ClusterKeycloakInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name: o.FromClusterInstance,
	}, instance); err != nil {
		return nil, fmt.Errorf("failed to get ClusterKeycloakInstance %s: %w", o.FromClusterInstance, err)
	}

	// Get credentials secret (namespace is required for cluster-scoped instances)
	if instance.Spec.Credentials.SecretRef.Namespace == "" {
		return nil, fmt.Errorf("ClusterKeycloakInstance %s has no namespace set for credentials secret", o.FromClusterInstance)
	}

	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretRef.Name,
		Namespace: instance.Spec.Credentials.SecretRef.Namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s/%s: %w", instance.Spec.Credentials.SecretRef.Namespace, instance.Spec.Credentials.SecretRef.Name, err)
	}

	usernameKey := "username"
	if instance.Spec.Credentials.SecretRef.UsernameKey != "" {
		usernameKey = instance.Spec.Credentials.SecretRef.UsernameKey
	}
	passwordKey := "password"
	if instance.Spec.Credentials.SecretRef.PasswordKey != "" {
		passwordKey = instance.Spec.Credentials.SecretRef.PasswordKey
	}

	username := string(secret.Data[usernameKey])
	password := string(secret.Data[passwordKey])

	if username == "" || password == "" {
		return nil, fmt.Errorf("credentials secret is missing username or password")
	}

	adminRealm := "master"
	if instance.Spec.Realm != nil {
		adminRealm = *instance.Spec.Realm
	}

	return &keycloak.Config{
		BaseURL:  instance.Spec.BaseUrl,
		Realm:    adminRealm,
		Username: username,
		Password: password,
	}, nil
}
