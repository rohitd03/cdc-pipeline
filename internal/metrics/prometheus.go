package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the application
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge

	// Kafka consumer metrics
	KafkaMessagesConsumedTotal *prometheus.CounterVec
	KafkaConsumerLag           *prometheus.GaugeVec
	KafkaProcessingErrors      *prometheus.CounterVec
	KafkaDLQMessages           prometheus.Counter
	KafkaProcessingDuration    *prometheus.HistogramVec

	// Elasticsearch metrics
	ESIndexOperationsTotal *prometheus.CounterVec
	ESOperationDuration    *prometheus.HistogramVec
	ESOperationErrors      *prometheus.CounterVec

	// Application metrics
	OrdersCreatedTotal         prometheus.Counter
	OrdersUpdatedTotal         prometheus.Counter
	OrdersDeletedTotal         prometheus.Counter
	PipelineLastEventTimestamp prometheus.Gauge
	PipelineLag                prometheus.Gauge
}

var (
	instance *Metrics
	once     sync.Once
)

// NewMetrics creates and registers all Prometheus metrics (singleton)
func NewMetrics() *Metrics {
	once.Do(func() {
		instance = &Metrics{
			// HTTP metrics
			HTTPRequestsTotal: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "http_requests_total",
					Help: "Total number of HTTP requests",
				},
				[]string{"method", "path", "status_code"},
			),
			HTTPRequestDuration: promauto.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "http_request_duration_seconds",
					Help:    "HTTP request latency in seconds",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"method", "path"},
			),
			HTTPRequestsInFlight: promauto.NewGauge(
				prometheus.GaugeOpts{
					Name: "http_requests_in_flight",
					Help: "Current number of HTTP requests being processed",
				},
			),

			// Kafka consumer metrics
			KafkaMessagesConsumedTotal: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "kafka_messages_consumed_total",
					Help: "Total number of Kafka messages consumed",
				},
				[]string{"topic", "partition"},
			),
			KafkaConsumerLag: promauto.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "kafka_consumer_lag",
					Help: "Current consumer lag for Kafka partitions",
				},
				[]string{"topic", "partition"},
			),
			KafkaProcessingErrors: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "kafka_processing_errors_total",
					Help: "Total number of Kafka message processing errors",
				},
				[]string{"topic", "error_type"},
			),
			KafkaDLQMessages: promauto.NewCounter(
				prometheus.CounterOpts{
					Name: "kafka_dlq_messages_total",
					Help: "Total number of messages sent to dead letter queue",
				},
			),
			KafkaProcessingDuration: promauto.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "kafka_message_processing_duration_seconds",
					Help:    "Time taken to process Kafka messages",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"topic"},
			),

			// Elasticsearch metrics
			ESIndexOperationsTotal: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "es_index_operations_total",
					Help: "Total number of Elasticsearch index operations",
				},
				[]string{"operation"}, // index, update, delete
			),
			ESOperationDuration: promauto.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "es_operation_duration_seconds",
					Help:    "Time taken for Elasticsearch operations",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"operation"},
			),
			ESOperationErrors: promauto.NewCounterVec(
				prometheus.CounterOpts{
					Name: "es_operation_errors_total",
					Help: "Total number of Elasticsearch operation errors",
				},
				[]string{"operation", "error_type"},
			),

			// Application metrics
			OrdersCreatedTotal: promauto.NewCounter(
				prometheus.CounterOpts{
					Name: "orders_created_total",
					Help: "Total number of orders created",
				},
			),
			OrdersUpdatedTotal: promauto.NewCounter(
				prometheus.CounterOpts{
					Name: "orders_updated_total",
					Help: "Total number of orders updated",
				},
			),
			OrdersDeletedTotal: promauto.NewCounter(
				prometheus.CounterOpts{
					Name: "orders_deleted_total",
					Help: "Total number of orders deleted",
				},
			),
			PipelineLastEventTimestamp: promauto.NewGauge(
				prometheus.GaugeOpts{
					Name: "pipeline_last_event_timestamp",
					Help: "Unix timestamp of the last processed CDC event",
				},
			),
			PipelineLag: promauto.NewGauge(
				prometheus.GaugeOpts{
					Name: "pipeline_lag_seconds",
					Help: "Pipeline lag in seconds (time behind real-time)",
				},
			),
		}
	})

	return instance
}
