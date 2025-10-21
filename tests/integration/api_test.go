//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rohit/cdc-pipeline/internal/api"
	"github.com/rohit/cdc-pipeline/internal/api/handler"
	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/db"
	"github.com/rohit/cdc-pipeline/internal/metrics"
	"github.com/rohit/cdc-pipeline/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Note: These tests require a running PostgreSQL instance
// Run with: docker compose -f docker-compose.test.yml up postgres-test

func TestAPI_CreateAndGetOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	// Setup database
	cfg := &config.PostgresConfig{
		Host:     "localhost",
		Port:     "5433",
		User:     "postgres",
		Password: "postgres",
		DBName:   "orders_db_test",
		SSLMode:  "disable",
		MaxConns: 5,
		MinConns: 1,
	}

	database, err := db.NewDB(ctx, cfg, logger)
	require.NoError(t, err)
	defer database.Close()

	err = database.RunMigrations(ctx)
	require.NoError(t, err)

	// Setup service and handlers
	metricsRegistry := metrics.NewMetrics()
	orderService := service.NewOrderService(database, metricsRegistry, logger)
	mockESRepo := new(MockESRepo)
	orderHandler := handler.NewOrderHandler(orderService, mockESRepo, logger)

	// Create router
	router := api.NewRouter(api.RouterConfig{
		OrderHandler: orderHandler,
		Metrics:      metricsRegistry,
		Logger:       logger,
	})

	// Test: Create order
	createReq := service.CreateOrderRequest{
		CustomerID:    uuid.New().String(),
		CustomerName:  "Integration Test User",
		CustomerEmail: "test@example.com",
		ProductName:   "Test Product",
		ProductSKU:    "TEST-SKU-001",
		Quantity:      2,
		Amount:        199.99,
		Currency:      "USD",
		ShippingAddr:  "123 Test Street",
		Notes:         "Integration test order",
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var createResp struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	err = json.NewDecoder(w.Body).Decode(&createResp)
	require.NoError(t, err)
	assert.True(t, createResp.Success)
	assert.NotEmpty(t, createResp.Data.ID)

	// Test: Get order
	req = httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+createResp.Data.ID, nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var getResp struct {
		Success bool `json:"success"`
		Data    struct {
			ID            string  `json:"id"`
			CustomerName  string  `json:"customer_name"`
			CustomerEmail string  `json:"customer_email"`
			ProductName   string  `json:"product_name"`
			Amount        float64 `json:"amount"`
			Status        string  `json:"status"`
		} `json:"data"`
	}

	err = json.NewDecoder(w.Body).Decode(&getResp)
	require.NoError(t, err)
	assert.True(t, getResp.Success)
	assert.Equal(t, createResp.Data.ID, getResp.Data.ID)
	assert.Equal(t, "Integration Test User", getResp.Data.CustomerName)
	assert.Equal(t, "pending", getResp.Data.Status)
}

func TestAPI_UpdateOrderStatus_ValidTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	// Setup database
	cfg := &config.PostgresConfig{
		Host:     "localhost",
		Port:     "5433",
		User:     "postgres",
		Password: "postgres",
		DBName:   "orders_db_test",
		SSLMode:  "disable",
		MaxConns: 5,
		MinConns: 1,
	}

	database, err := db.NewDB(ctx, cfg, logger)
	require.NoError(t, err)
	defer database.Close()

	// Setup service and handlers
	metricsRegistry := metrics.NewMetrics()
	orderService := service.NewOrderService(database, metricsRegistry, logger)
	mockESRepo := new(MockESRepo)
	orderHandler := handler.NewOrderHandler(orderService, mockESRepo, logger)

	router := api.NewRouter(api.RouterConfig{
		OrderHandler: orderHandler,
		Metrics:      metricsRegistry,
		Logger:       logger,
	})

	// Create an order first
	createReq := service.CreateOrderRequest{
		CustomerID:    uuid.New().String(),
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		ProductName:   "Test Product",
		ProductSKU:    "TEST-001",
		Quantity:      1,
		Amount:        100.00,
		Currency:      "USD",
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	var createResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)

	// Update status to confirmed
	updateReq := map[string]string{"status": "confirmed"}
	body, _ = json.Marshal(updateReq)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/orders/"+createResp.Data.ID+"/status", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updateResp struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}

	err = json.NewDecoder(w.Body).Decode(&updateResp)
	require.NoError(t, err)
	assert.True(t, updateResp.Success)
	assert.Equal(t, "confirmed", updateResp.Data.Status)
}

func TestAPI_DeleteOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	// Setup database
	cfg := &config.PostgresConfig{
		Host:     "localhost",
		Port:     "5433",
		User:     "postgres",
		Password: "postgres",
		DBName:   "orders_db_test",
		SSLMode:  "disable",
		MaxConns: 5,
		MinConns: 1,
	}

	database, err := db.NewDB(ctx, cfg, logger)
	require.NoError(t, err)
	defer database.Close()

	// Setup service and handlers
	metricsRegistry := metrics.NewMetrics()
	orderService := service.NewOrderService(database, metricsRegistry, logger)
	mockESRepo := new(MockESRepo)
	orderHandler := handler.NewOrderHandler(orderService, mockESRepo, logger)

	router := api.NewRouter(api.RouterConfig{
		OrderHandler: orderHandler,
		Metrics:      metricsRegistry,
		Logger:       logger,
	})

	// Create an order first
	createReq := service.CreateOrderRequest{
		CustomerID:    uuid.New().String(),
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		ProductName:   "Test Product",
		ProductSKU:    "TEST-001",
		Quantity:      1,
		Amount:        100.00,
		Currency:      "USD",
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	var createResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)

	// Delete the order
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/orders/"+createResp.Data.ID, nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify order is deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+createResp.Data.ID, nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
