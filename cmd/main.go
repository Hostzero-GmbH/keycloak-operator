package main

import (
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/controller"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(keycloakv1beta1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var syncPeriod time.Duration
	var maxConcurrentRequests int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&syncPeriod, "sync-period", controller.DefaultSyncPeriod,
		"The interval at which successfully reconciled resources are re-checked for drift. "+
			"Higher values reduce Keycloak API load but increase time to detect external changes.")
	flag.IntVar(&maxConcurrentRequests, "max-concurrent-requests", 10,
		"Maximum number of concurrent requests to Keycloak. Set to 0 for no limit. "+
			"Lower values reduce Keycloak load but increase reconciliation time.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Configure global sync period for all controllers
	controller.SetSyncPeriod(syncPeriod)
	setupLog.Info("configured sync period", "syncPeriod", syncPeriod)
	setupLog.Info("configured max concurrent requests", "maxConcurrentRequests", maxConcurrentRequests)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "keycloak-operator.hostzero.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create shared Keycloak client manager with rate limiting
	clientManager := keycloak.NewClientManagerWithConfig(ctrl.Log, keycloak.ClientManagerConfig{
		MaxConcurrentRequests: maxConcurrentRequests,
	})

	// Setup controllers
	if err = (&controller.KeycloakInstanceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakInstance")
		os.Exit(1)
	}

	if err = (&controller.KeycloakRealmReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakRealm")
		os.Exit(1)
	}

	if err = (&controller.KeycloakClientReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakClient")
		os.Exit(1)
	}

	if err = (&controller.KeycloakUserReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakUser")
		os.Exit(1)
	}

	if err = (&controller.KeycloakUserCredentialReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakUserCredential")
		os.Exit(1)
	}

	if err = (&controller.KeycloakRoleMappingReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakRoleMapping")
		os.Exit(1)
	}

	if err = (&controller.ClusterKeycloakInstanceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterKeycloakInstance")
		os.Exit(1)
	}

	if err = (&controller.ClusterKeycloakRealmReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterKeycloakRealm")
		os.Exit(1)
	}

	if err = (&controller.KeycloakClientScopeReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakClientScope")
		os.Exit(1)
	}

	if err = (&controller.KeycloakGroupReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakGroup")
		os.Exit(1)
	}

	if err = (&controller.KeycloakIdentityProviderReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakIdentityProvider")
		os.Exit(1)
	}

	if err = (&controller.KeycloakRoleReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakRole")
		os.Exit(1)
	}

	if err = (&controller.KeycloakProtocolMapperReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakProtocolMapper")
		os.Exit(1)
	}

	if err = (&controller.KeycloakComponentReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakComponent")
		os.Exit(1)
	}

	if err = (&controller.KeycloakOrganizationReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClientManager: clientManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KeycloakOrganization")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
