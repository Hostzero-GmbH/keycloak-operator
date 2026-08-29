// Package telemetry configures optional OpenTelemetry export.
//
// Export is enabled when OTEL_EXPORTER_OTLP_ENDPOINT or a signal-specific
// OTEL_EXPORTER_OTLP_{TRACES,LOGS}_ENDPOINT is set. All other OTEL_* env
// vars (protocol, sampler, headers, resource attributes) are honored by the SDK.
package telemetry

import (
	"context"
	"errors"
	"os"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.uber.org/zap/zapcore"
)

const ServiceName = "keycloak-operator"

var (
	tracingOn bool
	loggingOn bool
)

// LoggingEnabled reports whether OTLP log export was configured by Setup.
func LoggingEnabled() bool { return loggingOn }

// TracingEnabled reports whether OTLP trace export was configured by Setup.
func TracingEnabled() bool { return tracingOn }

// ZapCore returns a zap core that exports logs via OTLP.
// Only call when LoggingEnabled is true.
func ZapCore() zapcore.Core {
	return otelzap.NewCore(ServiceName)
}

// Setup initializes OpenTelemetry exporters from OTEL_* environment variables.
// It is a no-op when no OTLP endpoint is configured.
func Setup(ctx context.Context) (func(context.Context) error, error) {
	tracingOn = false
	loggingOn = false

	if !envEnabled() {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(serviceVersion()),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error

	if tracesEnabled() {
		exp, err := newTraceExporter(ctx)
		if err != nil {
			return nil, err
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		tracingOn = true
		shutdowns = append(shutdowns, tp.Shutdown)
	}

	if logsEnabled() {
		exp, err := newLogExporter(ctx)
		if err != nil {
			return shutdownAll(shutdowns), err
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
			sdklog.WithResource(res),
		)
		otellog.SetLoggerProvider(lp)
		loggingOn = true
		shutdowns = append(shutdowns, lp.Shutdown)
	}

	return shutdownAll(shutdowns), nil
}

func envEnabled() bool {
	return tracesEnabled() || logsEnabled()
}

func tracesEnabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

func logsEnabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != ""
}

func useHTTP() bool {
	p := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	return p == "http/protobuf" || p == "http/json"
}

func newTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if useHTTP() {
		return otlptracehttp.New(ctx)
	}
	return otlptracegrpc.New(ctx)
}

func newLogExporter(ctx context.Context) (sdklog.Exporter, error) {
	if useHTTP() {
		return otlploghttp.New(ctx)
	}
	return otlploggrpc.New(ctx)
}

func shutdownAll(fns []func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		var err error
		for i := len(fns) - 1; i >= 0; i-- {
			err = errors.Join(err, fns[i](ctx))
		}
		return err
	}
}

func serviceVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
		return v
	}
	return "dev"
}
