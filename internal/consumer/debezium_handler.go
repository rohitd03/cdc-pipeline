package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/model"
	"go.uber.org/zap"
)

// DebeziumHandler processes Debezium CDC events
type DebeziumHandler struct {
	esRepo  elasticsearch.OrderRepository
	metrics *metrics.Metrics
	logger  *zap.Logger
}

// NewDebeziumHandler creates a new DebeziumHandler
func NewDebeziumHandler(esRepo elasticsearch.OrderRepository, metrics *metrics.Metrics, logger *zap.Logger) *DebeziumHandler {
	return &DebeziumHandler{
		esRepo:  esRepo,
		metrics: metrics,
		logger:  logger,
	}
}

// HandleMessage processes a Debezium event message
func (h *DebeziumHandler) HandleMessage(ctx context.Context, messageBytes []byte) error {
	start := time.Now()

	// Parse Debezium event
	var event model.DebeziumEvent
	if err := json.Unmarshal(messageBytes, &event); err != nil {
		h.logger.Error("failed to parse debezium event",
			zap.Error(err),
			zap.String("message", string(messageBytes)),
		)
		return fmt.Errorf("debezium: failed to parse event: %w", err)
	}

	// Extract operation
	op := event.Payload.Op
	source := event.Payload.Source

	h.logger.Debug("processing debezium event",
		zap.String("op", op),
		zap.String("table", source.Table),
		zap.Int64("lsn", source.LSN),
	)

	// Route based on operation
	var err error
	switch op {
	case "c", "r": // create or read (snapshot)
		err = h.handleCreate(ctx, &event)
	case "u": // update
		err = h.handleUpdate(ctx, &event)
	case "d": // delete
		err = h.handleDelete(ctx, &event)
	default:
		h.logger.Warn("unknown operation",
			zap.String("op", op),
		)
		return fmt.Errorf("%w: %s", appErrors.ErrUnknownOperation, op)
	}

	if err != nil {
		return err
	}

	// Update pipeline metrics
	h.metrics.PipelineLastEventTimestamp.Set(float64(event.Payload.TsMs / 1000))
	lag := time.Now().Unix() - (event.Payload.TsMs / 1000)
	h.metrics.PipelineLag.Set(float64(lag))

	duration := time.Since(start).Seconds()
	h.logger.Info("debezium event processed",
		zap.String("op", op),
		zap.Float64("duration_seconds", duration),
	)

	return nil
}

// handleCreate processes INSERT operations
func (h *DebeziumHandler) handleCreate(ctx context.Context, event *model.DebeziumEvent) error {
	if event.Payload.After == nil {
		h.logger.Error("missing after field in create event")
		return fmt.Errorf("%w: create operation requires after field", appErrors.ErrMissingAfterField)
	}

	order := event.Payload.After.ToOrder()

	h.logger.Info("indexing new order",
		zap.String("order_id", order.ID),
		zap.String("status", string(order.Status)),
	)

	start := time.Now()
	if err := h.esRepo.IndexOrder(ctx, order); err != nil {
		h.metrics.ESOperationErrors.WithLabelValues("index", "elasticsearch_error").Inc()
		return fmt.Errorf("debezium: failed to index order: %w", err)
	}

	h.metrics.ESIndexOperationsTotal.WithLabelValues("index").Inc()
	h.metrics.ESOperationDuration.WithLabelValues("index").Observe(time.Since(start).Seconds())

	return nil
}

// handleUpdate processes UPDATE operations
func (h *DebeziumHandler) handleUpdate(ctx context.Context, event *model.DebeziumEvent) error {
	if event.Payload.After == nil {
		h.logger.Error("missing after field in update event")
		return fmt.Errorf("%w: update operation requires after field", appErrors.ErrMissingAfterField)
	}

	order := event.Payload.After.ToOrder()

	h.logger.Info("updating order",
		zap.String("order_id", order.ID),
		zap.String("status", string(order.Status)),
	)

	start := time.Now()
	if err := h.esRepo.UpdateOrder(ctx, order); err != nil {
		h.metrics.ESOperationErrors.WithLabelValues("update", "elasticsearch_error").Inc()
		return fmt.Errorf("debezium: failed to update order: %w", err)
	}

	h.metrics.ESIndexOperationsTotal.WithLabelValues("update").Inc()
	h.metrics.ESOperationDuration.WithLabelValues("update").Observe(time.Since(start).Seconds())

	return nil
}

// handleDelete processes DELETE operations
func (h *DebeziumHandler) handleDelete(ctx context.Context, event *model.DebeziumEvent) error {
	if event.Payload.Before == nil {
		h.logger.Error("missing before field in delete event")
		return fmt.Errorf("%w: delete operation requires before field", appErrors.ErrMissingBeforeField)
	}

	orderID := event.Payload.Before.ID

	h.logger.Info("deleting order",
		zap.String("order_id", orderID),
	)

	start := time.Now()
	if err := h.esRepo.DeleteOrder(ctx, orderID); err != nil {
		h.metrics.ESOperationErrors.WithLabelValues("delete", "elasticsearch_error").Inc()
		return fmt.Errorf("debezium: failed to delete order: %w", err)
	}

	h.metrics.ESIndexOperationsTotal.WithLabelValues("delete").Inc()
	h.metrics.ESOperationDuration.WithLabelValues("delete").Observe(time.Since(start).Seconds())

	return nil
}
