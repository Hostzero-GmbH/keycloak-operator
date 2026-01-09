package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsNamespace = "keycloak_operator"
)

var (
	// ReconcileTotal counts total reconciliations per controller and result
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "reconcile_total",
			Help:      "Total number of reconciliations per controller",
		},
		[]string{"controller", "result"},
	)

	// ReconcileDuration tracks reconciliation duration per controller
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"controller"},
	)

	// ReconcileErrors counts reconciliation errors per controller and error type
	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "reconcile_errors_total",
			Help:      "Total number of reconciliation errors per controller and error type",
		},
		[]string{"controller", "error_type"},
	)

	// ResourcesManaged tracks the number of resources being managed per type
	ResourcesManaged = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "resources_managed",
			Help:      "Number of resources currently being managed",
		},
		[]string{"resource_type", "namespace"},
	)

	// ResourcesReady tracks how many managed resources are in ready state
	ResourcesReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "resources_ready",
			Help:      "Number of resources in ready state",
		},
		[]string{"resource_type", "namespace"},
	)

	// KeycloakConnectionStatus tracks the connection status to Keycloak instances
	KeycloakConnectionStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "keycloak_connection_status",
			Help:      "Connection status to Keycloak instances (1=connected, 0=disconnected)",
		},
		[]string{"instance", "namespace"},
	)

	// KeycloakAPIRequestsTotal counts API requests to Keycloak
	KeycloakAPIRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "keycloak_api_requests_total",
			Help:      "Total number of API requests to Keycloak",
		},
		[]string{"instance", "method", "endpoint", "status"},
	)

	// KeycloakAPILatency tracks Keycloak API request latency
	KeycloakAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "keycloak_api_latency_seconds",
			Help:      "Latency of Keycloak API requests in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"instance", "method"},
	)

	// WorkQueueDepth tracks the depth of the controller work queue
	WorkQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "workqueue_depth",
			Help:      "Current depth of the controller work queue",
		},
		[]string{"controller"},
	)

	// LastReconcileTime tracks the last successful reconcile time
	LastReconcileTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "last_reconcile_timestamp_seconds",
			Help:      "Unix timestamp of last successful reconciliation",
		},
		[]string{"controller"},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		ReconcileErrors,
		ResourcesManaged,
		ResourcesReady,
		KeycloakConnectionStatus,
		KeycloakAPIRequestsTotal,
		KeycloakAPILatency,
		WorkQueueDepth,
		LastReconcileTime,
	)
}

// RecordReconcile records a reconciliation attempt
func RecordReconcile(controller string, success bool, duration float64) {
	result := "success"
	if !success {
		result = "error"
	}
	ReconcileTotal.WithLabelValues(controller, result).Inc()
	ReconcileDuration.WithLabelValues(controller).Observe(duration)
	if success {
		LastReconcileTime.WithLabelValues(controller).SetToCurrentTime()
	}
}

// RecordError records a reconciliation error
func RecordError(controller, errorType string) {
	ReconcileErrors.WithLabelValues(controller, errorType).Inc()
}

// SetResourceCounts updates the resource count gauges
func SetResourceCounts(resourceType, namespace string, managed, ready int) {
	ResourcesManaged.WithLabelValues(resourceType, namespace).Set(float64(managed))
	ResourcesReady.WithLabelValues(resourceType, namespace).Set(float64(ready))
}

// SetKeycloakConnectionStatus updates the Keycloak connection status
func SetKeycloakConnectionStatus(instance, namespace string, connected bool) {
	status := 0.0
	if connected {
		status = 1.0
	}
	KeycloakConnectionStatus.WithLabelValues(instance, namespace).Set(status)
}

// RecordKeycloakAPIRequest records a Keycloak API request
func RecordKeycloakAPIRequest(instance, method, endpoint, status string, latency float64) {
	KeycloakAPIRequestsTotal.WithLabelValues(instance, method, endpoint, status).Inc()
	KeycloakAPILatency.WithLabelValues(instance, method).Observe(latency)
}
