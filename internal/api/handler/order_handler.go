package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/rohit/cdc-pipeline/internal/api/middleware"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	appErrors "github.com/rohit/cdc-pipeline/internal/errors"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/rohit/cdc-pipeline/internal/service"
	"go.uber.org/zap"
)

// OrderHandler handles order-related HTTP requests
type OrderHandler struct {
	orderService service.OrderService
	esRepo       elasticsearch.OrderRepository
	validator    *validator.Validate
	logger       *zap.Logger
}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler(orderService service.OrderService, esRepo elasticsearch.OrderRepository, logger *zap.Logger) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
		esRepo:       esRepo,
		validator:    validator.New(),
		logger:       logger,
	}
}

// Response envelope for all API responses
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *ErrorInfo  `json:"error"`
	Meta    Meta        `json:"meta"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Meta contains response metadata
type Meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// CreateOrder handles POST /api/v1/orders
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON format", nil)
		return
	}

	// Validate request
	if err := h.validator.Struct(req); err != nil {
		validationErrors := h.formatValidationErrors(err)
		h.respondError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", validationErrors)
		return
	}

	order, err := h.orderService.CreateOrder(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to create order", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create order", nil)
		return
	}

	h.respondSuccess(w, r, http.StatusCreated, order)
}

// UpdateOrderStatus handles PUT /api/v1/orders/{id}/status
func (h *OrderHandler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Status model.OrderStatus `json:"status" validate:"required,oneof=pending confirmed shipped delivered cancelled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON format", nil)
		return
	}

	// Validate status value
	validStatuses := []model.OrderStatus{
		model.StatusPending,
		model.StatusConfirmed,
		model.StatusShipped,
		model.StatusDelivered,
		model.StatusCancelled,
	}
	isValid := false
	for _, status := range validStatuses {
		if req.Status == status {
			isValid = true
			break
		}
	}
	if !isValid {
		h.respondError(w, r, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value", nil)
		return
	}

	order, err := h.orderService.UpdateOrderStatus(r.Context(), id, req.Status)
	if err != nil {
		if errors.Is(err, appErrors.ErrOrderNotFound) {
			h.respondError(w, r, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found", nil)
			return
		}
		if errors.Is(err, appErrors.ErrInvalidStatusTransition) {
			h.respondError(w, r, http.StatusConflict, "INVALID_TRANSITION", "Invalid status transition", nil)
			return
		}
		h.logger.Error("failed to update order status", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update order", nil)
		return
	}

	h.respondSuccess(w, r, http.StatusOK, order)
}

// GetOrderByID handles GET /api/v1/orders/{id}
func (h *OrderHandler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	order, err := h.orderService.GetOrderByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, appErrors.ErrOrderNotFound) {
			h.respondError(w, r, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found", nil)
			return
		}
		h.logger.Error("failed to get order", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get order", nil)
		return
	}

	h.respondSuccess(w, r, http.StatusOK, order)
}

// DeleteOrder handles DELETE /api/v1/orders/{id}
func (h *OrderHandler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := h.orderService.DeleteOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, appErrors.ErrOrderNotFound) {
			h.respondError(w, r, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found", nil)
			return
		}
		h.logger.Error("failed to delete order", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete order", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SearchOrders handles GET /api/v1/search/orders
func (h *OrderHandler) SearchOrders(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse search parameters
	req := elasticsearch.SearchRequest{
		Query:      query.Get("q"),
		Status:     query.Get("status"),
		CustomerID: query.Get("customer_id"),
		SortBy:     query.Get("sort_by"),
		SortOrder:  query.Get("sort_order"),
	}

	// Parse numeric parameters
	if minAmount := query.Get("min_amount"); minAmount != "" {
		if val, err := strconv.ParseFloat(minAmount, 64); err == nil {
			req.MinAmount = val
		}
	}
	if maxAmount := query.Get("max_amount"); maxAmount != "" {
		if val, err := strconv.ParseFloat(maxAmount, 64); err == nil {
			req.MaxAmount = val
		}
	}

	// Parse pagination
	req.Page = 1
	if page := query.Get("page"); page != "" {
		if val, err := strconv.Atoi(page); err == nil && val > 0 {
			req.Page = val
		}
	}

	req.PageSize = 20
	if pageSize := query.Get("page_size"); pageSize != "" {
		if val, err := strconv.Atoi(pageSize); err == nil && val > 0 {
			req.PageSize = val
		}
	}

	// Parse date range
	if from := query.Get("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			req.From = t
		}
	}
	if to := query.Get("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			req.To = t
		}
	}

	result, err := h.esRepo.SearchOrders(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to search orders", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "SEARCH_ERROR", "Failed to search orders", nil)
		return
	}

	h.respondSuccess(w, r, http.StatusOK, result)
}

// GetOrderFromES handles GET /api/v1/search/orders/{id}
func (h *OrderHandler) GetOrderFromES(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	order, err := h.esRepo.GetOrderByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, appErrors.ErrOrderNotFound) ||
			(err.Error() == "elasticsearch: order not found") {
			h.respondError(w, r, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found", nil)
			return
		}
		h.logger.Error("failed to get order from ES", zap.Error(err))
		h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get order", nil)
		return
	}

	h.respondSuccess(w, r, http.StatusOK, order)
}

// respondSuccess sends a success response
func (h *OrderHandler) respondSuccess(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	response := Response{
		Success: true,
		Data:    data,
		Error:   nil,
		Meta: Meta{
			RequestID: middleware.GetRequestID(r.Context()),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// respondError sends an error response
func (h *OrderHandler) respondError(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	response := Response{
		Success: false,
		Data:    nil,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: Meta{
			RequestID: middleware.GetRequestID(r.Context()),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// formatValidationErrors formats validator errors into a readable format
func (h *OrderHandler) formatValidationErrors(err error) map[string]string {
	errors := make(map[string]string)

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			errors[e.Field()] = h.msgForTag(e)
		}
	}

	return errors
}

// msgForTag returns a human-readable message for validation tags
func (h *OrderHandler) msgForTag(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "email":
		return fe.Field() + " must be a valid email"
	case "uuid":
		return fe.Field() + " must be a valid UUID"
	case "min":
		return fe.Field() + " must be at least " + fe.Param()
	case "len":
		return fe.Field() + " must be " + fe.Param() + " characters"
	default:
		return fe.Field() + " is invalid"
	}
}
