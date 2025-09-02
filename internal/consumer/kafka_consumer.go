package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// MessageHandler defines the interface for processing messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, message []byte) error
}

// Consumer manages Kafka consumption
type Consumer struct {
	reader  *kafka.Reader
	writer  *kafka.Writer // for DLQ
	handler MessageHandler
	metrics *metrics.Metrics
	logger  *zap.Logger
	config  *config.KafkaConfig
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg *config.KafkaConfig, handler MessageHandler, metrics *metrics.Metrics, logger *zap.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
		Logger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(msg, args...))
		}),
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	})

	// DLQ writer
	writer := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers...),
		Topic:    cfg.DLQTopic,
		Balancer: &kafka.LeastBytes{},
		Logger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(msg, args...))
		}),
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error(fmt.Sprintf(msg, args...))
		}),
	}

	return &Consumer{
		reader:  reader,
		writer:  writer,
		handler: handler,
		metrics: metrics,
		logger:  logger,
		config:  cfg,
	}
}

// Start begins consuming messages
func (c *Consumer) Start(ctx context.Context) error {
	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Create cancellable context
	consumeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Goroutine to handle shutdown signal
	go func() {
		<-sigChan
		c.logger.Info("shutdown signal received, finishing current message processing")
		cancel()
	}()

	c.logger.Info("kafka consumer started",
		zap.String("topic", c.config.Topic),
		zap.String("group_id", c.config.GroupID),
		zap.Strings("brokers", c.config.Brokers),
	)

	// Main consumption loop
	for {
		select {
		case <-consumeCtx.Done():
			c.logger.Info("consumer context cancelled, shutting down")
			return c.Close()
		default:
			if err := c.consumeMessage(consumeCtx); err != nil {
				if err == context.Canceled {
					c.logger.Info("context cancelled during message consumption")
					return c.Close()
				}
				c.logger.Error("error consuming message", zap.Error(err))
				// Continue processing despite errors
			}
		}
	}
}

// consumeMessage reads and processes a single message
func (c *Consumer) consumeMessage(ctx context.Context) error {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		if err == context.Canceled {
			return err
		}
		c.metrics.KafkaProcessingErrors.WithLabelValues(c.config.Topic, "fetch_error").Inc()
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	c.logger.Debug("message received",
		zap.String("topic", msg.Topic),
		zap.Int("partition", msg.Partition),
		zap.Int64("offset", msg.Offset),
	)

	// Update metrics
	c.metrics.KafkaMessagesConsumedTotal.WithLabelValues(
		msg.Topic,
		fmt.Sprintf("%d", msg.Partition),
	).Inc()

	start := time.Now()

	// Process message with retry logic
	err = c.processWithRetry(ctx, msg.Value)

	// Record processing duration
	c.metrics.KafkaProcessingDuration.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())

	if err != nil {
		c.logger.Error("failed to process message after retries",
			zap.Error(err),
			zap.Int64("offset", msg.Offset),
		)

		// Send to DLQ
		if dlqErr := c.sendToDLQ(ctx, msg, err); dlqErr != nil {
			c.logger.Error("failed to send message to DLQ", zap.Error(dlqErr))
		}

		c.metrics.KafkaProcessingErrors.WithLabelValues(c.config.Topic, "processing_error").Inc()

		// Commit offset even on error to avoid reprocessing
		if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
			c.logger.Error("failed to commit offset", zap.Error(commitErr))
			return fmt.Errorf("failed to commit offset: %w", commitErr)
		}

		return err
	}

	// Commit offset after successful processing (at-least-once semantics)
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.logger.Error("failed to commit offset", zap.Error(err))
		return fmt.Errorf("failed to commit offset: %w", err)
	}

	return nil
}

// processWithRetry attempts to process a message with exponential backoff
func (c *Consumer) processWithRetry(ctx context.Context, message []byte) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(c.config.RetryBaseMs*(1<<(attempt-1))) * time.Millisecond
			c.logger.Warn("retrying message processing",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		err := c.handler.HandleMessage(ctx, message)
		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.Warn("message processing failed",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", c.config.MaxRetries),
		)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// sendToDLQ sends a failed message to the dead letter queue
func (c *Consumer) sendToDLQ(ctx context.Context, msg kafka.Message, processingErr error) error {
	dlqMessage := map[string]interface{}{
		"original_topic":     msg.Topic,
		"original_partition": msg.Partition,
		"original_offset":    msg.Offset,
		"original_key":       string(msg.Key),
		"original_value":     string(msg.Value),
		"error":              processingErr.Error(),
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
		"retry_count":        c.config.MaxRetries,
	}

	dlqBytes, err := json.Marshal(dlqMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal DLQ message: %w", err)
	}

	err = c.writer.WriteMessages(ctx, kafka.Message{
		Key:   msg.Key,
		Value: dlqBytes,
	})

	if err != nil {
		return fmt.Errorf("failed to write to DLQ: %w", err)
	}

	c.metrics.KafkaDLQMessages.Inc()

	c.logger.Info("message sent to DLQ",
		zap.String("topic", c.config.DLQTopic),
		zap.Int64("original_offset", msg.Offset),
	)

	return nil
}

// Close closes the consumer and DLQ writer
func (c *Consumer) Close() error {
	c.logger.Info("closing kafka consumer")

	var readerErr, writerErr error

	if err := c.reader.Close(); err != nil {
		c.logger.Error("failed to close kafka reader", zap.Error(err))
		readerErr = err
	}

	if err := c.writer.Close(); err != nil {
		c.logger.Error("failed to close kafka writer", zap.Error(err))
		writerErr = err
	}

	if readerErr != nil {
		return readerErr
	}
	if writerErr != nil {
		return writerErr
	}

	c.logger.Info("kafka consumer closed successfully")
	return nil
}

// Stats returns the current consumer statistics
func (c *Consumer) Stats() kafka.ReaderStats {
	return c.reader.Stats()
}
