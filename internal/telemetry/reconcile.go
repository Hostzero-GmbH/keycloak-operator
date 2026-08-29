package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const instrumentationName = "github.com/Hostzero-GmbH/keycloak-operator"

// WrapReconciler returns a reconciler that starts a root span per reconcile
// and injects trace_id/span_id into the context logger. When tracing is
// disabled it returns r unchanged.
func WrapReconciler(name string, r reconcile.Reconciler) reconcile.Reconciler {
	if !tracingOn {
		return r
	}
	return &reconcilerWrapper{name: name, next: r}
}

type reconcilerWrapper struct {
	name string
	next reconcile.Reconciler
}

func (w *reconcilerWrapper) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx, "Reconcile",
		trace.WithAttributes(
			attribute.String("controller", w.name),
			attribute.String("k8s.namespace.name", req.Namespace),
			attribute.String("k8s.name", req.Name),
		),
	)
	defer span.End()

	if sc := span.SpanContext(); sc.IsValid() {
		ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
			"trace_id", sc.TraceID().String(),
			"span_id", sc.SpanID().String(),
		))
	}

	result, err := w.next.Reconcile(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}
