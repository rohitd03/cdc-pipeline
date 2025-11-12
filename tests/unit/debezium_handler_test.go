package unit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rohit/cdc-pipeline/internal/consumer"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockOrderRepository is a mock implementation of OrderRepository
type MockOrderRepository struct {
	mock.Mock
}

func (m *MockOrderRepository) IndexOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockOrderRepository) UpdateOrder(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockOrderRepository) DeleteOrder(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOrderRepository) SearchOrders(ctx context.Context, req elasticsearch.SearchRequest) (*elasticsearch.SearchResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*elasticsearch.SearchResult), args.Error(1)
}

func (m *MockOrderRepository) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func TestDebeziumHandler_HandleInsertEvent(t *testing.T) {
	tests := []struct {
		name        string
		event       model.DebeziumPayload
		expectError bool
		errorType   error
		setupMock   func(*MockOrderRepository)
	}{
		{
			name: "valid INSERT event",
			event: model.DebeziumPayload{
				Op: "c",
				After: &model.OrderDBRow{
					ID:            "order-1",
					CustomerID:    "cust-1",
					CustomerName:  "John Doe",
					CustomerEmail: "john@example.com",
					ProductName:   "Laptop",
					ProductSKU:    "LT-001",
					Quantity:      1,
					Amount:        1000.00,
					Currency:      "USD",
					Status:        "pending",
					ShippingAddr:  "123 Main St",
					Notes:         "Test order",
					CreatedAt:     time.Now().Format(time.RFC3339Nano),
					UpdatedAt:     time.Now().Format(time.RFC3339Nano),
				},
				Source: model.DebeziumSource{
					Table: "orders",
					LSN:   12345,
				},
				TsMs: time.Now().UnixMilli(),
			},
			expectError: false,
			setupMock: func(m *MockOrderRepository) {
				m.On("IndexOrder", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil)
			},
		},
		{
			name: "missing after field in INSERT event",
			event: model.DebeziumPayload{
				Op:    "c",
				After: nil,
				Source: model.DebeziumSource{
					Table: "orders",
				},
				TsMs: time.Now().UnixMilli(),
			},
			expectError: true,
			errorType:   appErrors.ErrMissingAfterField,
			setupMock:   func(m *MockOrderRepository) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockOrderRepository)
			tt.setupMock(mockRepo)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

			eventBytes, _ := json.Marshal(tt.event)
			err := handler.HandleMessage(context.Background(), eventBytes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestDebeziumHandler_HandleUpdateEvent(t *testing.T) {
	tests := []struct {
		name        string
		event       model.DebeziumPayload
		expectError bool
		setupMock   func(*MockOrderRepository)
	}{
		{
			name: "valid UPDATE event",
			event: model.DebeziumPayload{
				Op: "u",
				After: &model.OrderDBRow{
					ID:            "order-1",
					CustomerID:    "cust-1",
					CustomerName:  "John Doe",
					CustomerEmail: "john@example.com",
					ProductName:   "Laptop",
					ProductSKU:    "LT-001",
					Quantity:      1,
					Amount:        1000.00,
					Currency:      "USD",
					Status:        "confirmed",
					ShippingAddr:  "123 Main St",
					Notes:         "Test order",
					CreatedAt:     time.Now().Format(time.RFC3339Nano),
					UpdatedAt:     time.Now().Format(time.RFC3339Nano),
				},
				Source: model.DebeziumSource{
					Table: "orders",
					LSN:   12346,
				},
				TsMs: time.Now().UnixMilli(),
			},
			expectError: false,
			setupMock: func(m *MockOrderRepository) {
				m.On("UpdateOrder", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockOrderRepository)
			tt.setupMock(mockRepo)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

			eventBytes, _ := json.Marshal(tt.event)
			err := handler.HandleMessage(context.Background(), eventBytes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestDebeziumHandler_HandleDeleteEvent(t *testing.T) {
	tests := []struct {
		name        string
		event       model.DebeziumPayload
		expectError bool
		errorType   error
		setupMock   func(*MockOrderRepository)
	}{
		{
			name: "valid DELETE event",
			event: model.DebeziumPayload{
				Op: "d",
				Before: &model.OrderDBRow{
					ID: "order-1",
				},
				Source: model.DebeziumSource{
					Table: "orders",
					LSN:   12347,
				},
				TsMs: time.Now().UnixMilli(),
			},
			expectError: false,
			setupMock: func(m *MockOrderRepository) {
				m.On("DeleteOrder", mock.Anything, "order-1").Return(nil)
			},
		},
		{
			name: "missing before field in DELETE event",
			event: model.DebeziumPayload{
				Op:     "d",
				Before: nil,
				Source: model.DebeziumSource{
					Table: "orders",
				},
				TsMs: time.Now().UnixMilli(),
			},
			expectError: true,
			errorType:   appErrors.ErrMissingBeforeField,
			setupMock:   func(m *MockOrderRepository) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockOrderRepository)
			tt.setupMock(mockRepo)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

			eventBytes, _ := json.Marshal(tt.event)
			err := handler.HandleMessage(context.Background(), eventBytes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestDebeziumHandler_HandleSnapshotEvent(t *testing.T) {
	mockRepo := new(MockOrderRepository)
	mockRepo.On("IndexOrder", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	// Snapshot events have op="r"
	event := model.DebeziumPayload{
		Op: "r",
		After: &model.OrderDBRow{
			ID:            "order-1",
			CustomerID:    "cust-1",
			CustomerName:  "John Doe",
			CustomerEmail: "john@example.com",
			ProductName:   "Laptop",
			ProductSKU:    "LT-001",
			Quantity:      1,
			Amount:        1000.00,
			Currency:      "USD",
			Status:        "pending",
			ShippingAddr:  "123 Main St",
			Notes:         "Test order",
			CreatedAt:     time.Now().Format(time.RFC3339Nano),
			UpdatedAt:     time.Now().Format(time.RFC3339Nano),
		},
		Source: model.DebeziumSource{
			Table: "orders",
		},
		TsMs: time.Now().UnixMilli(),
	}

	eventBytes, _ := json.Marshal(event)
	err := handler.HandleMessage(context.Background(), eventBytes)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestDebeziumHandler_HandleUnknownOperation(t *testing.T) {
	mockRepo := new(MockOrderRepository)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	event := model.DebeziumPayload{
		Op: "x", // Unknown operation
		Source: model.DebeziumSource{
			Table: "orders",
		},
		TsMs: time.Now().UnixMilli(),
	}

	eventBytes, _ := json.Marshal(event)
	err := handler.HandleMessage(context.Background(), eventBytes)

	assert.Error(t, err)
	assert.ErrorIs(t, err, appErrors.ErrUnknownOperation)
}

func TestDebeziumHandler_HandleMalformedJSON(t *testing.T) {
	mockRepo := new(MockOrderRepository)

	logger, _ := zap.NewDevelopment()
	metricsRegistry := metrics.NewMetrics()

	handler := consumer.NewDebeziumHandler(mockRepo, metricsRegistry, logger)

	malformedJSON := []byte(`{"invalid json`)

	err := handler.HandleMessage(context.Background(), malformedJSON)

	assert.Error(t, err)
}
