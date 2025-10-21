package unit

import (
	"context"
	"testing"

	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/rohit/cdc-pipeline/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockDB is a mock implementation of the DB interface
type MockDB struct {
	mock.Mock
}

func (m *MockDB) CreateOrder(ctx context.Context, order *model.Order) (*model.Order, error) {
	args := m.Called(ctx, order)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockDB) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	args := m.Called(ctx, id, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockDB) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockDB) ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockDB) DeleteOrder(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDB) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockDB) Close() {
	m.Called()
}

func TestOrderService_CreateOrder(t *testing.T) {
	tests := []struct {
		name        string
		request     service.CreateOrderRequest
		expectError bool
		setupMock   func(*MockDB)
	}{
		{
			name: "valid order creation",
			request: service.CreateOrderRequest{
				CustomerID:    "123e4567-e89b-12d3-a456-426614174000",
				CustomerName:  "John Doe",
				CustomerEmail: "john@example.com",
				ProductName:   "Laptop",
				ProductSKU:    "LT-001",
				Quantity:      1,
				Amount:        1000.00,
				Currency:      "USD",
				ShippingAddr:  "123 Main St",
				Notes:         "Test order",
			},
			expectError: false,
			setupMock: func(m *MockDB) {
				m.On("CreateOrder", mock.Anything, mock.AnythingOfType("*model.Order")).
					Return(&model.Order{
						ID:            "order-123",
						CustomerID:    "123e4567-e89b-12d3-a456-426614174000",
						CustomerName:  "John Doe",
						CustomerEmail: "john@example.com",
						ProductName:   "Laptop",
						ProductSKU:    "LT-001",
						Quantity:      1,
						Amount:        1000.00,
						Currency:      "USD",
						Status:        model.StatusPending,
						ShippingAddr:  "123 Main St",
						Notes:         "Test order",
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(MockDB)
			tt.setupMock(mockDB)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			// MockDB already implements db.DBInterface
			svc := service.NewOrderService(mockDB, metricsRegistry, logger)

			order, err := svc.CreateOrder(context.Background(), tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, order)
				assert.Equal(t, model.StatusPending, order.Status)
				assert.Equal(t, "USD", order.Currency)
				mockDB.AssertExpectations(t)
			}
		})
	}
}

func TestOrderService_UpdateOrderStatus(t *testing.T) {
	tests := []struct {
		name          string
		orderID       string
		newStatus     model.OrderStatus
		currentStatus model.OrderStatus
		expectError   bool
		errorType     error
		setupMock     func(*MockDB)
	}{
		{
			name:          "valid transition pending to confirmed",
			orderID:       "order-123",
			newStatus:     model.StatusConfirmed,
			currentStatus: model.StatusPending,
			expectError:   false,
			setupMock: func(m *MockDB) {
				m.On("GetOrderByID", mock.Anything, "order-123").
					Return(&model.Order{
						ID:     "order-123",
						Status: model.StatusPending,
					}, nil)
				m.On("UpdateOrderStatus", mock.Anything, "order-123", model.StatusConfirmed).
					Return(&model.Order{
						ID:     "order-123",
						Status: model.StatusConfirmed,
					}, nil)
			},
		},
		{
			name:          "invalid transition delivered to pending",
			orderID:       "order-123",
			newStatus:     model.StatusPending,
			currentStatus: model.StatusDelivered,
			expectError:   true,
			errorType:     appErrors.ErrInvalidStatusTransition,
			setupMock: func(m *MockDB) {
				m.On("GetOrderByID", mock.Anything, "order-123").
					Return(&model.Order{
						ID:     "order-123",
						Status: model.StatusDelivered,
					}, nil)
			},
		},
		{
			name:        "order not found",
			orderID:     "nonexistent",
			newStatus:   model.StatusConfirmed,
			expectError: true,
			errorType:   appErrors.ErrOrderNotFound,
			setupMock: func(m *MockDB) {
				m.On("GetOrderByID", mock.Anything, "nonexistent").
					Return(nil, appErrors.ErrOrderNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(MockDB)
			tt.setupMock(mockDB)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			// MockDB already implements db.DBInterface
			svc := service.NewOrderService(mockDB, metricsRegistry, logger)

			order, err := svc.UpdateOrderStatus(context.Background(), tt.orderID, tt.newStatus)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, order)
				assert.Equal(t, tt.newStatus, order.Status)
				mockDB.AssertExpectations(t)
			}
		})
	}
}

func TestOrderService_DeleteOrder(t *testing.T) {
	tests := []struct {
		name        string
		orderID     string
		expectError bool
		errorType   error
		setupMock   func(*MockDB)
	}{
		{
			name:        "successful delete",
			orderID:     "order-123",
			expectError: false,
			setupMock: func(m *MockDB) {
				m.On("DeleteOrder", mock.Anything, "order-123").Return(nil)
			},
		},
		{
			name:        "order not found",
			orderID:     "nonexistent",
			expectError: true,
			errorType:   appErrors.ErrOrderNotFound,
			setupMock: func(m *MockDB) {
				m.On("DeleteOrder", mock.Anything, "nonexistent").
					Return(appErrors.ErrOrderNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(MockDB)
			tt.setupMock(mockDB)

			logger, _ := zap.NewDevelopment()
			metricsRegistry := metrics.NewMetrics()

			// MockDB already implements db.DBInterface
			svc := service.NewOrderService(mockDB, metricsRegistry, logger)

			err := svc.DeleteOrder(context.Background(), tt.orderID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				mockDB.AssertExpectations(t)
			}
		})
	}
}
