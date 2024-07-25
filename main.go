package main

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
	"log"
	"net/http"
)

// Sample database model
type User struct {
	ID   uint   `gorm:"primarykey"`
	Name string `gorm:"index"`
}

func initTracer() func() {
	exporter, err := otlptrace.New(
		context.Background(),
		otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint("localhost:4317"), // SigNoz OTLP endpoint
			otlptracehttp.WithInsecure(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("example-service"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)

	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}
}

func main() {
	// Initialize SigNoz tracer
	shutdown := initTracer()
	defer shutdown()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(otelecho.Middleware("example-service"))

	// Initialize OpenTelemetry Tracer
	tracer := otel.Tracer("example-service")

	// PostgreSQL DSN (Data Source Name)
	dsn := "host=35.244.14.125 dbname=test-vishnu user=postgres password=dev-superleap sslmode=disable"

	// Initialize GORM with PostgreSQL and OpenTelemetry plugin
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := db.Use(tracing.NewPlugin(tracing.WithTracerProvider(otel.GetTracerProvider()))); err != nil {
		log.Fatalf("failed to set up tracing plugin: %v", err)
	}

	// Migrate the schema
	db.AutoMigrate(&User{})

	// Handlers
	e.GET("/users", func(c echo.Context) error {
		// Start a new span for the database operation
		ctx, span := tracer.Start(c.Request().Context(), "Fetch Users")
		defer span.End()

		var users []User
		if err := db.WithContext(ctx).Find(&users).Error; err != nil {
			return err
		}

		return c.JSON(http.StatusOK, users)
	})

	e.POST("/users", func(c echo.Context) error {
		// Start a new span for the database operation
		ctx, span := tracer.Start(c.Request().Context(), "Create User")
		defer span.End()

		user := User{Name: "John Doe"}
		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			return err
		}

		return c.JSON(http.StatusOK, user)
	})

	// Start server
	e.Logger.Fatal(e.Start(":8080"))
}
