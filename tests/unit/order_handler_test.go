package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rohit/cdc-pipeline/internal/api/handler"
	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/rohit/cdc-pipeline/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockOrderService is a mock implementation of OrderService
type MockOrderService struct {
	mock.Mock
}

func (m *MockOrderService) CreateOrder(ctx context.Context, req service.CreateOrderRequest) (*model.Order, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	args := m.Called(ctx, id, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderService) DeleteOrder(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestOrderHandler_CreateOrder(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		setupMock      func(*MockOrderService)
	}{
		{
			name: "valid order creation",
			requestBody: service.CreateOrderRequest{
				CustomerID:    "123e4567-e89b-12d3-a456-426614174000",
				CustomerName:  "John Doe",
				CustomerEmail: "john@example.com",
				ProductName:   "Laptop",
				ProductSKU:    "LT-001",
				Quantity:      1,
				Amount:        1000.00,
				Currency:      "USD",
			},
			expectedStatus: http.StatusCreated,
			setupMock: func(m *MockOrderService) {
				m.On("CreateOrder", mock.Anything, mock.AnythingOfType("service.CreateOrderRequest")).
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
					}, nil)
			},
		},
		{
			name: "missing required field",
			requestBody: map[string]interface{}{
				"customer_id": "123e4567-e89b-12d3-a456-426614174000",
				// Missing customer_name
				"customer_email": "john@example.com",
			},
			expectedStatus: http.StatusBadRequest,
			setupMock:      func(m *MockOrderService) {},
		},
		{
			name:           "invalid JSON",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			setupMock:      func(m *MockOrderService) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := new(MockOrderService)
			mockESRepo := new(MockOrderRepository)
			tt.setupMock(mockService)

			logger, _ := zap.NewDevelopment()
			h := handler.NewOrderHandler(mockService, mockESRepo, logger)

			var body []byte
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.CreateOrder(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestOrderHandler_UpdateOrderStatus(t *testing.T) {
	tests := []struct {
		name           string
		orderID        string
		requestBody    interface{}
		expectedStatus int
		setupMock      func(*MockOrderService)
	}{
		{
			name:    "valid status update",
			orderID: "order-123",
			requestBody: map[string]string{
				"status": "confirmed",
			},
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockOrderService) {
				m.On("UpdateOrderStatus", mock.Anything, "order-123", model.StatusConfirmed).
					Return(&model.Order{
						ID:     "order-123",
						Status: model.StatusConfirmed,
					}, nil)
			},
		},
		{
			name:    "invalid status transition",
			orderID: "order-123",
			requestBody: map[string]string{
				"status": "pending",
			},
			expectedStatus: http.StatusConflict,
			setupMock: func(m *MockOrderService) {
				m.On("UpdateOrderStatus", mock.Anything, "order-123", model.StatusPending).
					Return(nil, appErrors.ErrInvalidStatusTransition)
			},
		},
		{
			name:    "order not found",
			orderID: "nonexistent",
			requestBody: map[string]string{
				"status": "confirmed",
			},
			expectedStatus: http.StatusNotFound,
			setupMock: func(m *MockOrderService) {
				m.On("UpdateOrderStatus", mock.Anything, "nonexistent", model.StatusConfirmed).
					Return(nil, appErrors.ErrOrderNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := new(MockOrderService)
			mockESRepo := new(MockOrderRepository)
			tt.setupMock(mockService)

			logger, _ := zap.NewDevelopment()
			h := handler.NewOrderHandler(mockService, mockESRepo, logger)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/orders/"+tt.orderID+"/status", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Setup chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.orderID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.UpdateOrderStatus(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestOrderHandler_DeleteOrder(t *testing.T) {
	tests := []struct {
		name           string
		orderID        string
		expectedStatus int
		setupMock      func(*MockOrderService)
	}{
		{
			name:           "successful delete",
			orderID:        "order-123",
			expectedStatus: http.StatusNoContent,
			setupMock: func(m *MockOrderService) {
				m.On("DeleteOrder", mock.Anything, "order-123").Return(nil)
			},
		},
		{
			name:           "order not found",
			orderID:        "nonexistent",
			expectedStatus: http.StatusNotFound,
			setupMock: func(m *MockOrderService) {
				m.On("DeleteOrder", mock.Anything, "nonexistent").
					Return(appErrors.ErrOrderNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := new(MockOrderService)
			mockESRepo := new(MockOrderRepository)
			tt.setupMock(mockService)

			logger, _ := zap.NewDevelopment()
			h := handler.NewOrderHandler(mockService, mockESRepo, logger)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/orders/"+tt.orderID, nil)
			w := httptest.NewRecorder()

			// Setup chi URL params
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.orderID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			h.DeleteOrder(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
