//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rohit/cdc-pipeline/internal/consumer"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockESRepo for integration testing
type MockESRepo struct {
	mock.Mock
}

func (m *MockESRepo) IndexOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockESRepo) UpdateOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockESRepo) DeleteOrder(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockESRepo) SearchOrders(ctx context.Context, req elasticsearch.SearchRequest) (*elasticsearch.SearchResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*elasticsearch.SearchResult), args.Error(1)
}

func (m *MockESRepo) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func TestConsumer_ProcessDebeziumInsertMessage(t *testing.T) {
	mockRepo := new(MockESRepo)
	mockRepo.On("IndexOrder", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	// Create a Debezium INSERT event
	event := model.DebeziumEvent{
		Payload: model.DebeziumPayload{
			Op: "c",
			After: &model.OrderDBRow{
				ID:            "test-order-1",
				CustomerID:    "cust-1",
				CustomerName:  "John Doe",
				CustomerEmail: "john@example.com",
				ProductName:   "Test Product",
				ProductSKU:    "TEST-001",
				Quantity:      2,
				Amount:        99.99,
				Currency:      "USD",
				Status:        "pending",
				ShippingAddr:  "123 Test St",
				Notes:         "Test note",
				CreatedAt:     time.Now().UnixMicro(),
				UpdatedAt:     time.Now().UnixMicro(),
			},
			Source: model.DebeziumSource{
				Table: "orders",
				LSN:   1000,
			},
			TsMs: time.Now().UnixMilli(),
		},
	}

	eventBytes, err := json.Marshal(event)
	assert.NoError(t, err)

	// Process the message
	err = handler.HandleMessage(context.Background(), eventBytes)
	assert.NoError(t, err)

	// Verify IndexOrder was called
	mockRepo.AssertExpectations(t)
}

func TestConsumer_ProcessDebeziumUpdateMessage(t *testing.T) {
	mockRepo := new(MockESRepo)
	mockRepo.On("UpdateOrder", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	// Create a Debezium UPDATE event
	event := model.DebeziumEvent{
		Payload: model.DebeziumPayload{
			Op: "u",
			Before: &model.OrderDBRow{
				ID:     "test-order-1",
				Status: "pending",
			},
			After: &model.OrderDBRow{
				ID:            "test-order-1",
				CustomerID:    "cust-1",
				CustomerName:  "John Doe",
				CustomerEmail: "john@example.com",
				ProductName:   "Test Product",
				ProductSKU:    "TEST-001",
				Quantity:      2,
				Amount:        99.99,
				Currency:      "USD",
				Status:        "confirmed",
				ShippingAddr:  "123 Test St",
				Notes:         "Test note",
				CreatedAt:     time.Now().UnixMicro(),
				UpdatedAt:     time.Now().UnixMicro(),
			},
			Source: model.DebeziumSource{
				Table: "orders",
				LSN:   1001,
			},
			TsMs: time.Now().UnixMilli(),
		},
	}

	eventBytes, err := json.Marshal(event)
	assert.NoError(t, err)

	// Process the message
	err = handler.HandleMessage(context.Background(), eventBytes)
	assert.NoError(t, err)

	// Verify UpdateOrder was called
	mockRepo.AssertExpectations(t)
}

func TestConsumer_ProcessDebeziumDeleteMessage(t *testing.T) {
	mockRepo := new(MockESRepo)
	mockRepo.On("DeleteOrder", mock.Anything, "test-order-1").Return(nil)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	// Create a Debezium DELETE event
	event := model.DebeziumEvent{
		Payload: model.DebeziumPayload{
			Op: "d",
			Before: &model.OrderDBRow{
				ID: "test-order-1",
			},
			Source: model.DebeziumSource{
				Table: "orders",
				LSN:   1002,
			},
			TsMs: time.Now().UnixMilli(),
		},
	}

	eventBytes, err := json.Marshal(event)
	assert.NoError(t, err)

	// Process the message
	err = handler.HandleMessage(context.Background(), eventBytes)
	assert.NoError(t, err)

	// Verify DeleteOrder was called
	mockRepo.AssertExpectations(t)
}
