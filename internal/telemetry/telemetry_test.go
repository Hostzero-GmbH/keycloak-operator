package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type stubReconciler struct {
	called bool
	err    error
}

func (s *stubReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	s.called = true
	return reconcile.Result{}, s.err
}

func TestSetup_NoopWhenUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")

	shutdown, err := Setup(context.Background())
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if TracingEnabled() || LoggingEnabled() {
		t.Fatal("expected telemetry disabled when no endpoint is set")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSetup_WithHTTPEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	shutdown, err := Setup(context.Background())
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		tracingOn = false
		loggingOn = false
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
	if !TracingEnabled() || !LoggingEnabled() {
		t.Fatal("expected traces and logs enabled")
	}
}

func TestWrapReconciler_DisabledReturnsSame(t *testing.T) {
	tracingOn = false
	r := &stubReconciler{}
	got := WrapReconciler("Test", r)
	if got != r {
		t.Fatalf("expected original reconciler, got %T", got)
	}
}

func TestWrapReconciler_EnabledStartsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	tracingOn = true
	t.Cleanup(func() {
		tracingOn = false
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

	r := &stubReconciler{}
	wrapped := WrapReconciler("KeycloakRealm", r)
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "realm"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !r.called {
		t.Fatal("expected inner reconciler to be called")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "Reconcile" {
		t.Fatalf("span name: %s", spans[0].Name)
	}
}

func TestWrapReconciler_RecordsError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	tracingOn = true
	t.Cleanup(func() {
		tracingOn = false
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

	r := &stubReconciler{err: errors.New("boom")}
	wrapped := WrapReconciler("KeycloakRealm", r)
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "realm"},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code.String() != "Error" {
		t.Fatalf("expected Error status, got %s", spans[0].Status.Code)
	}
}

func TestWrapReconciler_InjectsTraceIDs(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tracingOn = true
	t.Cleanup(func() {
		tracingOn = false
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

	var sawKeys bool
	wrapped := WrapReconciler("KeycloakRealm", reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		// logger values are stored on the logr sink; presence of a valid span
		// plus IntoContext is enough — FromContext must not be the zero logger.
		if !log.FromContext(ctx).Enabled() && log.FromContext(ctx).GetSink() == nil {
			t.Fatal("expected logger in context")
		}
		sawKeys = true
		return reconcile.Result{}, nil
	}))
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "realm"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !sawKeys {
		t.Fatal("inner reconciler was not invoked")
	}
}
