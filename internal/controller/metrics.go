package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconcileTotal counts the total number of reconciliations
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "keycloak_operator_reconcile_total",
			Help: "Total number of reconciliations per controller",
		},
		[]string{"controller", "result"},
	)

	// ReconcileDuration tracks the duration of reconciliations
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "keycloak_operator_reconcile_duration_seconds",
			Help:    "Duration of reconciliations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	// KeycloakAPIRequests counts Keycloak API requests
	KeycloakAPIRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "keycloak_operator_api_requests_total",
			Help: "Total number of Keycloak API requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	// ManagedResources tracks the number of managed resources
	ManagedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "keycloak_operator_managed_resources",
			Help: "Number of managed Keycloak resources",
		},
		[]string{"kind"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		KeycloakAPIRequests,
		ManagedResources,
	)
}
