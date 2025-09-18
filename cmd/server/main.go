package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rohit/cdc-pipeline/internal/api"
	"github.com/rohit/cdc-pipeline/internal/api/handler"
	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/consumer"
	"github.com/rohit/cdc-pipeline/internal/db"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/service"
	"go.uber.org/zap"
)

func main() {
	// Load .env file if it exists (ignore error in production)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := initLogger(cfg.Log.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("starting cdc-pipeline service",
		zap.String("service", "cdc-pipeline"),
		zap.String("log_level", cfg.Log.Level),
	)

	// Initialize metrics
	metricsRegistry := metrics.NewMetrics()

	// Initialize database
	ctx := context.Background()
	database, err := db.NewDB(ctx, &cfg.Postgres, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.Close()

	// Run migrations
	logger.Info("running database migrations")
	if err := database.RunMigrations(ctx); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	// Initialize Elasticsearch
	esClient, err := elasticsearch.NewClient(&cfg.Elasticsearch, logger)
	if err != nil {
		logger.Fatal("failed to connect to elasticsearch", zap.Error(err))
	}

	esRepo := elasticsearch.NewOrderRepository(esClient, cfg.Elasticsearch.IndexName)

	// Initialize services
	orderService := service.NewOrderService(database, metricsRegistry, logger)

	// Initialize Debezium handler
	debeziumHandler := consumer.NewDebeziumHandler(esRepo, metricsRegistry, logger)

	// Initialize Kafka consumer
	kafkaConsumer := consumer.NewConsumer(&cfg.Kafka, debeziumHandler, metricsRegistry, logger)

	// Start Kafka consumer in a goroutine
	consumerErrChan := make(chan error, 1)
	go func() {
		logger.Info("starting kafka consumer")
		if err := kafkaConsumer.Start(ctx); err != nil {
			consumerErrChan <- err
		}
	}()

	// Initialize HTTP handlers
	orderHandler := handler.NewOrderHandler(orderService, esRepo, logger)
	healthHandler := handler.NewHealthHandler(database, esClient, kafkaConsumer, logger)
	pipelineStatusHandler := handler.NewPipelineStatusHandler(kafkaConsumer, logger)

	// Setup router
	router := api.NewRouter(api.RouterConfig{
		OrderHandler:          orderHandler,
		HealthHandler:         healthHandler,
		PipelineStatusHandler: pipelineStatusHandler,
		Metrics:               metricsRegistry,
		Logger:                logger,
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start HTTP server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		logger.Info("starting http server",
			zap.String("addr", srv.Addr),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrChan <- err
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrChan:
		logger.Fatal("http server error", zap.Error(err))
	case err := <-consumerErrChan:
		logger.Fatal("kafka consumer error", zap.Error(err))
	case sig := <-sigChan:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	// Graceful shutdown
	logger.Info("shutting down gracefully")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", zap.Error(err))
	}

	logger.Info("service stopped")
}

// initLogger initializes the zap logger
func initLogger(level string) (*zap.Logger, error) {
	var zapLevel zap.AtomicLevel
	switch level {
	case "debug":
		zapLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapLevel = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapLevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	config := zap.Config{
		Level:            zapLevel,
		Development:      false,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return config.Build()
}
