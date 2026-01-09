package controller

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordReconcile_Success(t *testing.T) {
	// Reset metrics for test isolation
	ReconcileTotal.Reset()
	ReconcileDuration.Reset()
	LastReconcileTime.Reset()

	RecordReconcile("TestController", true, 0.5)

	// Verify counter incremented with correct labels
	count := testutil.ToFloat64(ReconcileTotal.WithLabelValues("TestController", "success"))
	if count != 1 {
		t.Errorf("expected reconcile_total=1, got %v", count)
	}

	// Verify error counter not incremented
	errorCount := testutil.ToFloat64(ReconcileTotal.WithLabelValues("TestController", "error"))
	if errorCount != 0 {
		t.Errorf("expected error count=0, got %v", errorCount)
	}

	// Verify last reconcile time was set (should be > 0)
	lastTime := testutil.ToFloat64(LastReconcileTime.WithLabelValues("TestController"))
	if lastTime == 0 {
		t.Error("expected last_reconcile_timestamp to be set")
	}
}

func TestRecordReconcile_Failure(t *testing.T) {
	ReconcileTotal.Reset()
	LastReconcileTime.Reset()

	RecordReconcile("TestController", false, 1.0)

	// Verify error counter incremented
	count := testutil.ToFloat64(ReconcileTotal.WithLabelValues("TestController", "error"))
	if count != 1 {
		t.Errorf("expected error reconcile_total=1, got %v", count)
	}

	// Verify last reconcile time was NOT set on failure
	lastTime := testutil.ToFloat64(LastReconcileTime.WithLabelValues("TestController"))
	if lastTime != 0 {
		t.Error("expected last_reconcile_timestamp to NOT be set on failure")
	}
}

func TestRecordReconcile_Duration(t *testing.T) {
	ReconcileDuration.Reset()

	RecordReconcile("TestController", true, 2.5)

	// Verify histogram was observed by checking it can be collected
	// We use testutil.CollectAndCount which is simpler
	count := testutil.CollectAndCount(ReconcileDuration)
	if count == 0 {
		t.Error("expected duration histogram to have observations")
	}
}

func TestRecordError(t *testing.T) {
	ReconcileErrors.Reset()

	RecordError("TestController", "connection_error")
	RecordError("TestController", "connection_error")
	RecordError("TestController", "fetch_error")

	connErrors := testutil.ToFloat64(ReconcileErrors.WithLabelValues("TestController", "connection_error"))
	if connErrors != 2 {
		t.Errorf("expected connection_error=2, got %v", connErrors)
	}

	fetchErrors := testutil.ToFloat64(ReconcileErrors.WithLabelValues("TestController", "fetch_error"))
	if fetchErrors != 1 {
		t.Errorf("expected fetch_error=1, got %v", fetchErrors)
	}
}

func TestSetResourceCounts(t *testing.T) {
	ResourcesManaged.Reset()
	ResourcesReady.Reset()

	SetResourceCounts("KeycloakRealm", "default", 10, 8)

	managed := testutil.ToFloat64(ResourcesManaged.WithLabelValues("KeycloakRealm", "default"))
	if managed != 10 {
		t.Errorf("expected managed=10, got %v", managed)
	}

	ready := testutil.ToFloat64(ResourcesReady.WithLabelValues("KeycloakRealm", "default"))
	if ready != 8 {
		t.Errorf("expected ready=8, got %v", ready)
	}
}

func TestSetKeycloakConnectionStatus(t *testing.T) {
	KeycloakConnectionStatus.Reset()

	// Test connected
	SetKeycloakConnectionStatus("my-instance", "default", true)
	status := testutil.ToFloat64(KeycloakConnectionStatus.WithLabelValues("my-instance", "default"))
	if status != 1 {
		t.Errorf("expected connection status=1 (connected), got %v", status)
	}

	// Test disconnected
	SetKeycloakConnectionStatus("my-instance", "default", false)
	status = testutil.ToFloat64(KeycloakConnectionStatus.WithLabelValues("my-instance", "default"))
	if status != 0 {
		t.Errorf("expected connection status=0 (disconnected), got %v", status)
	}
}

func TestRecordKeycloakAPIRequest(t *testing.T) {
	KeycloakAPIRequestsTotal.Reset()
	KeycloakAPILatency.Reset()

	RecordKeycloakAPIRequest("my-instance", "GET", "/realms", "200", 0.1)
	RecordKeycloakAPIRequest("my-instance", "GET", "/realms", "200", 0.2)
	RecordKeycloakAPIRequest("my-instance", "POST", "/users", "201", 0.3)

	getRequests := testutil.ToFloat64(KeycloakAPIRequestsTotal.WithLabelValues("my-instance", "GET", "/realms", "200"))
	if getRequests != 2 {
		t.Errorf("expected GET requests=2, got %v", getRequests)
	}

	postRequests := testutil.ToFloat64(KeycloakAPIRequestsTotal.WithLabelValues("my-instance", "POST", "/users", "201"))
	if postRequests != 1 {
		t.Errorf("expected POST requests=1, got %v", postRequests)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// This test verifies all metrics can be collected without panicking
	// and have the expected metric names

	metrics := []struct {
		name      string
		collector prometheus.Collector
	}{
		{"keycloak_operator_reconcile_total", ReconcileTotal},
		{"keycloak_operator_reconcile_duration_seconds", ReconcileDuration},
		{"keycloak_operator_reconcile_errors_total", ReconcileErrors},
		{"keycloak_operator_resources_managed", ResourcesManaged},
		{"keycloak_operator_resources_ready", ResourcesReady},
		{"keycloak_operator_keycloak_connection_status", KeycloakConnectionStatus},
		{"keycloak_operator_keycloak_api_requests_total", KeycloakAPIRequestsTotal},
		{"keycloak_operator_keycloak_api_latency_seconds", KeycloakAPILatency},
		{"keycloak_operator_workqueue_depth", WorkQueueDepth},
		{"keycloak_operator_last_reconcile_timestamp_seconds", LastReconcileTime},
	}

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			// Verify metric can be described (registered correctly)
			ch := make(chan *prometheus.Desc, 10)
			m.collector.Describe(ch)
			close(ch)

			found := false
			for desc := range ch {
				if strings.Contains(desc.String(), m.name) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("metric %s not found in descriptions", m.name)
			}
		})
	}
}

func TestControllerLabels(t *testing.T) {
	// Verify the expected controller names work correctly
	controllers := []string{
		"KeycloakInstance",
		"KeycloakRealm",
		"KeycloakClient",
		"KeycloakUser",
	}

	ReconcileTotal.Reset()

	for _, controller := range controllers {
		RecordReconcile(controller, true, 0.1)
	}

	for _, controller := range controllers {
		count := testutil.ToFloat64(ReconcileTotal.WithLabelValues(controller, "success"))
		if count != 1 {
			t.Errorf("expected %s count=1, got %v", controller, count)
		}
	}
}

func TestErrorTypes(t *testing.T) {
	// Verify all documented error types can be recorded
	errorTypes := []string{
		"fetch_error",
		"connection_error",
		"instance_not_ready",
		"realm_not_ready",
		"invalid_definition",
		"keycloak_api_error",
		"secret_sync_error",
	}

	ReconcileErrors.Reset()

	for _, errType := range errorTypes {
		RecordError("TestController", errType)
	}

	for _, errType := range errorTypes {
		count := testutil.ToFloat64(ReconcileErrors.WithLabelValues("TestController", errType))
		if count != 1 {
			t.Errorf("expected error type %s count=1, got %v", errType, count)
		}
	}
}
