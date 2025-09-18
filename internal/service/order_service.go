package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rohit/cdc-pipeline/internal/db"
	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/model"
	"go.uber.org/zap"
)

// OrderService defines business logic operations for orders
type OrderService interface {
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*model.Order, error)
	UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error)
	GetOrderByID(ctx context.Context, id string) (*model.Order, error)
	ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error)
	DeleteOrder(ctx context.Context, id string) error
}

// orderService implements OrderService
type orderService struct {
	db      db.DBInterface
	metrics *metrics.Metrics
	logger  *zap.Logger
}

// NewOrderService creates a new OrderService
func NewOrderService(database db.DBInterface, metrics *metrics.Metrics, logger *zap.Logger) OrderService {
	return &orderService{
		db:      database,
		metrics: metrics,
		logger:  logger,
	}
}

// CreateOrderRequest contains fields for creating a new order
type CreateOrderRequest struct {
	CustomerID    string  `json:"customer_id" validate:"required,uuid"`
	CustomerName  string  `json:"customer_name" validate:"required"`
	CustomerEmail string  `json:"customer_email" validate:"required,email"`
	ProductName   string  `json:"product_name" validate:"required"`
	ProductSKU    string  `json:"product_sku" validate:"required"`
	Quantity      int     `json:"quantity" validate:"required,min=1"`
	Amount        float64 `json:"amount" validate:"required,min=0"`
	Currency      string  `json:"currency" validate:"len=3"`
	ShippingAddr  string  `json:"shipping_address"`
	Notes         string  `json:"notes"`
}

// CreateOrder creates a new order
func (s *orderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*model.Order, error) {
	// Set defaults
	if req.Currency == "" {
		req.Currency = "USD"
	}

	// Generate order ID
	orderID := uuid.New().String()

	order := &model.Order{
		ID:            orderID,
		CustomerID:    req.CustomerID,
		CustomerName:  req.CustomerName,
		CustomerEmail: req.CustomerEmail,
		ProductName:   req.ProductName,
		ProductSKU:    req.ProductSKU,
		Quantity:      req.Quantity,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Status:        model.StatusPending,
		ShippingAddr:  req.ShippingAddr,
		Notes:         req.Notes,
	}

	createdOrder, err := s.db.CreateOrder(ctx, order)
	if err != nil {
		s.logger.Error("failed to create order",
			zap.Error(err),
			zap.String("customer_id", req.CustomerID),
		)
		return nil, fmt.Errorf("service: %w", err)
	}

	s.metrics.OrdersCreatedTotal.Inc()

	s.logger.Info("order created",
		zap.String("order_id", createdOrder.ID),
		zap.String("customer_id", createdOrder.CustomerID),
		zap.String("status", string(createdOrder.Status)),
	)

	return createdOrder, nil
}

// UpdateOrderStatus updates an order's status with validation
func (s *orderService) UpdateOrderStatus(ctx context.Context, id string, newStatus model.OrderStatus) (*model.Order, error) {
	// Get current order
	currentOrder, err := s.db.GetOrderByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get order for status update",
			zap.Error(err),
			zap.String("order_id", id),
		)
		return nil, fmt.Errorf("service: %w", appErrors.ErrOrderNotFound)
	}

	// Validate status transition
	if !isValidStatusTransition(currentOrder.Status, newStatus) {
		s.logger.Warn("invalid status transition",
			zap.String("order_id", id),
			zap.String("current_status", string(currentOrder.Status)),
			zap.String("new_status", string(newStatus)),
		)
		return nil, appErrors.ErrInvalidStatusTransition
	}

	updatedOrder, err := s.db.UpdateOrderStatus(ctx, id, newStatus)
	if err != nil {
		s.logger.Error("failed to update order status",
			zap.Error(err),
			zap.String("order_id", id),
		)
		return nil, fmt.Errorf("service: %w", err)
	}

	s.metrics.OrdersUpdatedTotal.Inc()

	s.logger.Info("order status updated",
		zap.String("order_id", id),
		zap.String("old_status", string(currentOrder.Status)),
		zap.String("new_status", string(newStatus)),
	)

	return updatedOrder, nil
}

// GetOrderByID retrieves an order by ID
func (s *orderService) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	order, err := s.db.GetOrderByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get order",
			zap.Error(err),
			zap.String("order_id", id),
		)
		return nil, fmt.Errorf("service: %w", appErrors.ErrOrderNotFound)
	}

	return order, nil
}

// ListOrders retrieves a paginated list of orders
func (s *orderService) ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error) {
	// Set sensible defaults and limits
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	orders, err := s.db.ListOrders(ctx, limit, offset)
	if err != nil {
		s.logger.Error("failed to list orders",
			zap.Error(err),
			zap.Int("limit", limit),
			zap.Int("offset", offset),
		)
		return nil, fmt.Errorf("service: %w", err)
	}

	return orders, nil
}

// DeleteOrder deletes an order by ID
func (s *orderService) DeleteOrder(ctx context.Context, id string) error {
	err := s.db.DeleteOrder(ctx, id)
	if err != nil {
		s.logger.Error("failed to delete order",
			zap.Error(err),
			zap.String("order_id", id),
		)
		return fmt.Errorf("service: %w", appErrors.ErrOrderNotFound)
	}

	s.metrics.OrdersDeletedTotal.Inc()

	s.logger.Info("order deleted",
		zap.String("order_id", id),
	)

	return nil
}

// isValidStatusTransition checks if a status transition is allowed
func isValidStatusTransition(current, new model.OrderStatus) bool {
	validTransitions := map[model.OrderStatus][]model.OrderStatus{
		model.StatusPending:   {model.StatusConfirmed, model.StatusCancelled},
		model.StatusConfirmed: {model.StatusShipped, model.StatusCancelled},
		model.StatusShipped:   {model.StatusDelivered},
		model.StatusDelivered: {}, // Terminal state
		model.StatusCancelled: {}, // Terminal state
	}

	allowedNext, exists := validTransitions[current]
	if !exists {
		return false
	}

	for _, allowed := range allowedNext {
		if allowed == new {
			return true
		}
	}

	return false
}
