package api

import (
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rohit/cdc-pipeline/internal/api/handler"
	"github.com/rohit/cdc-pipeline/internal/api/middleware"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"go.uber.org/zap"
)

// RouterConfig contains dependencies for setting up the router
type RouterConfig struct {
	OrderHandler          *handler.OrderHandler
	HealthHandler         *handler.HealthHandler
	PipelineStatusHandler *handler.PipelineStatusHandler
	Metrics               *metrics.Metrics
	Logger                *zap.Logger
}

// NewRouter creates and configures the Chi router
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Apply global middleware
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.Logging(cfg.Logger))
	r.Use(middleware.Metrics(cfg.Metrics))

	// Health check endpoint (no prefix)
	r.Get("/health", cfg.HealthHandler.Health)

	// Metrics endpoint (no prefix)
	r.Handle("/metrics", promhttp.Handler())

	// API routes under /api/v1
	r.Route("/api/v1", func(r chi.Router) {
		// Order management (write operations - Postgres)
		r.Route("/orders", func(r chi.Router) {
			r.Post("/", cfg.OrderHandler.CreateOrder)
			r.Get("/{id}", cfg.OrderHandler.GetOrderByID)
			r.Put("/{id}/status", cfg.OrderHandler.UpdateOrderStatus)
			r.Delete("/{id}", cfg.OrderHandler.DeleteOrder)
		})

		// Search operations (read operations - Elasticsearch)
		r.Route("/search/orders", func(r chi.Router) {
			r.Get("/", cfg.OrderHandler.SearchOrders)
			r.Get("/{id}", cfg.OrderHandler.GetOrderFromES)
		})

		// Pipeline status
		r.Get("/pipeline/status", cfg.PipelineStatusHandler.PipelineStatus)
	})

	return r
}
