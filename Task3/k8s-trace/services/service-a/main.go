// service-a/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const serviceName = "service-a"

func initTracer() (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	// Экспортер в Jaeger через OTLP/HTTP (Collector)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("jaeger-collector:4318"),
		otlptracehttp.WithInsecure(), // отключён TLS — для dev
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}

func callServiceB(ctx context.Context) error {
	tracer := otel.Tracer(serviceName)
	ctx, span := tracer.Start(ctx, "call-service-b")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://service-b:8080/api/order", nil)
	if err != nil {
		span.RecordError(err)
		return err
	}

	client := http.Client{
		Timeout:   5 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("service-b returned %d", resp.StatusCode)
		span.RecordError(err)
		span.SetAttributes(attribute.String("http.status_code", fmt.Sprintf("%d", resp.StatusCode)))
		return err
	}

	span.SetAttributes(attribute.Bool("service_b.success", true))
	return nil
}

func main() {
	tp, err := initTracer()
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	r := gin.New()
	r.Use(otelgin.Middleware(serviceName))

	r.GET("/api/calculate", func(c *gin.Context) {
		ctx := c.Request.Context()
		_, span := otel.Tracer(serviceName).Start(ctx, "handle-calculate")
		defer span.End()

		span.AddEvent("Calling service-b")

		if err := callServiceB(ctx); err != nil {
			span.RecordError(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		span.AddEvent("Service-b call succeeded")
		c.JSON(http.StatusOK, gin.H{
			"message": "Calculation done, order created in service-b",
		})
	})

	log.Println("Service A starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
