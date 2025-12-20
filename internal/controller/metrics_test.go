package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMetricsRegistration(t *testing.T) {
	// Verify metrics are registered
	assert.NotNil(t, ReconcileTotal)
	assert.NotNil(t, ReconcileDuration)
	assert.NotNil(t, KeycloakAPIRequests)
	assert.NotNil(t, ManagedResources)
}

func TestReconcileTotal(t *testing.T) {
	// Test that we can increment the counter
	ReconcileTotal.With(prometheus.Labels{
		"controller": "test",
		"result":     "success",
	}).Inc()

	// Should not panic
}

func TestReconcileDuration(t *testing.T) {
	// Test that we can observe durations
	ReconcileDuration.With(prometheus.Labels{
		"controller": "test",
	}).Observe(0.5)

	// Should not panic
}

func TestManagedResources(t *testing.T) {
	// Test that we can set gauge
	ManagedResources.With(prometheus.Labels{
		"kind": "KeycloakRealm",
	}).Set(5)

	// Should not panic
}
