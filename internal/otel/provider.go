package otel

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OTel configuration
type Config struct {
	Enabled      bool
	ServiceName  string
	BatchTimeout time.Duration
	LogWriter    io.Writer // File to write OTel logs to (required when enabled)
	Endpoint     string    // OTLP endpoint (optional, only used if set)
	Insecure     bool      // Use insecure connection for OTLP
}

// Provider manages OpenTelemetry providers for logs and metrics
type Provider struct {
	logProvider *sdklog.LoggerProvider
	config      Config
}

// New creates a new OTel provider with the given configuration.
// If OTel is disabled, returns a no-op provider.
func New(cfg Config) (*Provider, error) {
	p := &Provider{
		config: cfg,
	}

	if !cfg.Enabled {
		return p, nil
	}

	ctx := context.Background()

	// Create resource with service name
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var processors []sdklog.Processor

	// Always set up file-based log exporter when enabled
	if cfg.LogWriter != nil {
		fileExporter, err := stdoutlog.New(
			stdoutlog.WithWriter(cfg.LogWriter),
			stdoutlog.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create file log exporter: %w", err)
		}
		processors = append(processors, sdklog.NewBatchProcessor(fileExporter,
			sdklog.WithExportTimeout(cfg.BatchTimeout),
		))
	}

	// Optionally set up OTLP exporter if endpoint is configured
	if cfg.Endpoint != "" {
		otlpOpts := []otlploghttp.Option{
			otlploghttp.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			otlpOpts = append(otlpOpts, otlploghttp.WithInsecure())
		}

		otlpExporter, err := otlploghttp.New(ctx, otlpOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP log exporter: %w", err)
		}
		processors = append(processors, sdklog.NewBatchProcessor(otlpExporter,
			sdklog.WithExportTimeout(cfg.BatchTimeout),
		))
	}

	if len(processors) == 0 {
		return nil, fmt.Errorf("OTel enabled but no log writer or endpoint configured")
	}

	// Create log provider with all processors
	opts := []sdklog.LoggerProviderOption{
		sdklog.WithResource(res),
	}
	for _, proc := range processors {
		opts = append(opts, sdklog.WithProcessor(proc))
	}
	p.logProvider = sdklog.NewLoggerProvider(opts...)

	return p, nil
}

// LoggerProvider returns the log provider for use with otelslog bridge.
// Returns nil if OTel is not enabled.
func (p *Provider) LoggerProvider() *sdklog.LoggerProvider {
	return p.logProvider
}

// Meter returns a meter with the given name for creating metrics.
// Returns a no-op meter since we're using file-based logging primarily.
func (p *Provider) Meter(name string) metric.Meter {
	return noop.Meter{}
}

// Flush forces a flush of all pending logs.
// Use this during mission save to ensure all data is exported.
func (p *Provider) Flush(ctx context.Context) error {
	if !p.config.Enabled {
		return nil
	}

	if p.logProvider != nil {
		if err := p.logProvider.ForceFlush(ctx); err != nil {
			return fmt.Errorf("log flush failed: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully shuts down all providers.
// Should be called when the application exits.
func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.config.Enabled {
		return nil
	}

	if p.logProvider != nil {
		if err := p.logProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("log shutdown failed: %w", err)
		}
	}

	return nil
}

// Enabled returns whether OTel is enabled
func (p *Provider) Enabled() bool {
	return p.config.Enabled
}

// ensure otel import is used
var _ = otel.Version
