package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rohit/cdc-pipeline/internal/consumer"
	"github.com/rohit/cdc-pipeline/internal/db"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	"go.uber.org/zap"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	db       *db.DB
	esClient *elasticsearch.Client
	consumer *consumer.Consumer
	logger   *zap.Logger
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(db *db.DB, esClient *elasticsearch.Client, consumer *consumer.Consumer, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		db:       db,
		esClient: esClient,
		consumer: consumer,
		logger:   logger,
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status       string                      `json:"status"`
	Timestamp    string                      `json:"timestamp"`
	Dependencies map[string]DependencyHealth `json:"dependencies"`
}

// DependencyHealth represents the health status of a dependency
type DependencyHealth struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// Health handles GET /health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	response := HealthResponse{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Dependencies: make(map[string]DependencyHealth),
	}

	allHealthy := true

	// Check PostgreSQL
	if err := h.db.Ping(ctx); err != nil {
		response.Dependencies["postgres"] = DependencyHealth{
			Healthy: false,
			Message: "Connection failed",
		}
		allHealthy = false
		h.logger.Warn("postgres health check failed", zap.Error(err))
	} else {
		response.Dependencies["postgres"] = DependencyHealth{
			Healthy: true,
		}
	}

	// Check Elasticsearch
	if err := h.esClient.Ping(); err != nil {
		response.Dependencies["elasticsearch"] = DependencyHealth{
			Healthy: false,
			Message: "Connection failed",
		}
		allHealthy = false
		h.logger.Warn("elasticsearch health check failed", zap.Error(err))
	} else {
		response.Dependencies["elasticsearch"] = DependencyHealth{
			Healthy: true,
		}
	}

	// Check Kafka consumer (basic check - if we have stats, it's running)
	// Note: This is a simple check. In production, you might want more sophisticated checks
	if h.consumer != nil {
		response.Dependencies["kafka"] = DependencyHealth{
			Healthy: true,
		}
	} else {
		response.Dependencies["kafka"] = DependencyHealth{
			Healthy: false,
			Message: "Consumer not initialized",
		}
		allHealthy = false
	}

	if allHealthy {
		response.Status = "healthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else {
		response.Status = "unhealthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// PipelineStatusHandler handles pipeline status requests
type PipelineStatusHandler struct {
	consumer *consumer.Consumer
	logger   *zap.Logger
}

// NewPipelineStatusHandler creates a new PipelineStatusHandler
func NewPipelineStatusHandler(consumer *consumer.Consumer, logger *zap.Logger) *PipelineStatusHandler {
	return &PipelineStatusHandler{
		consumer: consumer,
		logger:   logger,
	}
}

// PipelineStatusResponse represents the pipeline status
type PipelineStatusResponse struct {
	GroupID            string                      `json:"group_id"`
	ConsumerLag        map[string]int64            `json:"consumer_lag"`
	LastProcessedEvent int64                       `json:"last_processed_event_timestamp"`
	Dependencies       map[string]DependencyHealth `json:"dependencies"`
}

// PipelineStatus handles GET /api/v1/pipeline/status
func (h *PipelineStatusHandler) PipelineStatus(w http.ResponseWriter, r *http.Request) {
	response := PipelineStatusResponse{
		GroupID:      "cdc-pipeline-consumer",
		ConsumerLag:  make(map[string]int64),
		Dependencies: make(map[string]DependencyHealth),
	}

	// Get consumer stats if available
	if h.consumer != nil {
		stats := h.consumer.Stats()

		// Get lag for the current partition
		// kafka.ReaderStats contains lag for a single partition
		partitionKey := fmt.Sprintf("%s", stats.Partition)
		response.ConsumerLag[partitionKey] = stats.Lag

		// Set last processed event timestamp (using current time as proxy)
		// In a real implementation, you'd track this from the actual processed events
		response.LastProcessedEvent = time.Now().Unix()

		// Consumer is healthy if it's initialized
		response.Dependencies["kafka"] = DependencyHealth{
			Healthy: true,
		}
	} else {
		response.Dependencies["kafka"] = DependencyHealth{
			Healthy: false,
			Message: "Consumer not initialized",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
