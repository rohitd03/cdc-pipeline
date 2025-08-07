package errors

import "errors"

// Sentinel errors for the application
var (
	ErrOrderNotFound            = errors.New("order not found")
	ErrInvalidStatusTransition  = errors.New("invalid status transition")
	ErrDuplicateOrder           = errors.New("order already exists")
	ErrElasticsearchUnavailable = errors.New("elasticsearch unavailable")
	ErrKafkaUnavailable         = errors.New("kafka unavailable")
	ErrDatabaseUnavailable      = errors.New("database unavailable")
	ErrInvalidInput             = errors.New("invalid input")
	ErrUnknownOperation         = errors.New("unknown operation")
	ErrMissingAfterField        = errors.New("missing after field in debezium event")
	ErrMissingBeforeField       = errors.New("missing before field in debezium event")
)
