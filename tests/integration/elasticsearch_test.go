//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/elasticsearch"
	"github.com/rohit/cdc-pipeline/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Note: These tests require a running Elasticsearch instance
// Run with: docker compose -f docker-compose.test.yml up elasticsearch-test

// setupTestIndex creates an index with proper mappings for testing
func setupTestIndex(ctx context.Context, client *elasticsearch.Client, indexName string) error {
	mapping := map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id":               map[string]string{"type": "keyword"},
				"customer_id":      map[string]string{"type": "keyword"},
				"customer_name":    map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": map[string]string{"type": "keyword"}}},
				"customer_email":   map[string]string{"type": "keyword"},
				"product_name":     map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": map[string]string{"type": "keyword"}}},
				"product_sku":      map[string]string{"type": "keyword"},
				"quantity":         map[string]string{"type": "integer"},
				"amount":           map[string]string{"type": "double"},
				"currency":         map[string]string{"type": "keyword"},
				"status":           map[string]string{"type": "keyword"},
				"shipping_address": map[string]string{"type": "text"},
				"notes":            map[string]string{"type": "text"},
				"created_at":       map[string]string{"type": "date"},
				"updated_at":       map[string]string{"type": "date"},
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(mapping); err != nil {
		return err
	}

	res, err := client.GetClient().Indices.Create(
		indexName,
		client.GetClient().Indices.Create.WithContext(ctx),
		client.GetClient().Indices.Create.WithBody(&buf),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil // Index might already exist, which is fine
	}

	return nil
}

func TestElasticsearch_IndexAndGetOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	cfg := &config.ElasticsearchConfig{
		Addresses: []string{"http://localhost:9201"},
		IndexName: "test_orders_" + uuid.New().String()[:8],
	}

	client, err := elasticsearch.NewClient(cfg, logger)
	require.NoError(t, err)

	// Setup index with proper mappings
	err = setupTestIndex(ctx, client, cfg.IndexName)
	require.NoError(t, err)

	repo := elasticsearch.NewOrderRepository(client, cfg.IndexName)

	// Create a test order
	order := &model.Order{
		ID:            uuid.New().String(),
		CustomerID:    uuid.New().String(),
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
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Index the order
	err = repo.IndexOrder(ctx, order)
	assert.NoError(t, err)

	// Small delay for indexing
	time.Sleep(1 * time.Second)

	// Retrieve the order
	retrieved, err := repo.GetOrderByID(ctx, order.ID)
	assert.NoError(t, err)
	assert.Equal(t, order.ID, retrieved.ID)
	assert.Equal(t, order.CustomerName, retrieved.CustomerName)
	assert.Equal(t, order.Amount, retrieved.Amount)
}

func TestElasticsearch_UpdateOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	cfg := &config.ElasticsearchConfig{
		Addresses: []string{"http://localhost:9201"},
		IndexName: "test_orders_" + uuid.New().String()[:8],
	}

	client, err := elasticsearch.NewClient(cfg, logger)
	require.NoError(t, err)

	// Setup index with proper mappings
	err = setupTestIndex(ctx, client, cfg.IndexName)
	require.NoError(t, err)

	repo := elasticsearch.NewOrderRepository(client, cfg.IndexName)

	// Create and index an order
	order := &model.Order{
		ID:            uuid.New().String(),
		CustomerID:    uuid.New().String(),
		CustomerName:  "Jane Doe",
		CustomerEmail: "jane@example.com",
		ProductName:   "Phone",
		ProductSKU:    "PH-001",
		Quantity:      1,
		Amount:        500.00,
		Currency:      "USD",
		Status:        model.StatusPending,
		ShippingAddr:  "456 Oak Ave",
		Notes:         "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = repo.IndexOrder(ctx, order)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Update the order
	order.Status = model.StatusConfirmed
	order.UpdatedAt = time.Now()

	err = repo.UpdateOrder(ctx, order)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Verify update
	retrieved, err := repo.GetOrderByID(ctx, order.ID)
	assert.NoError(t, err)
	assert.Equal(t, model.StatusConfirmed, retrieved.Status)
}

func TestElasticsearch_DeleteOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	cfg := &config.ElasticsearchConfig{
		Addresses: []string{"http://localhost:9201"},
		IndexName: "test_orders_" + uuid.New().String()[:8],
	}

	client, err := elasticsearch.NewClient(cfg, logger)
	require.NoError(t, err)

	// Setup index with proper mappings
	err = setupTestIndex(ctx, client, cfg.IndexName)
	require.NoError(t, err)

	repo := elasticsearch.NewOrderRepository(client, cfg.IndexName)

	// Create and index an order
	order := &model.Order{
		ID:            uuid.New().String(),
		CustomerID:    uuid.New().String(),
		CustomerName:  "Bob Smith",
		CustomerEmail: "bob@example.com",
		ProductName:   "Monitor",
		ProductSKU:    "MON-001",
		Quantity:      1,
		Amount:        300.00,
		Currency:      "USD",
		Status:        model.StatusPending,
		ShippingAddr:  "789 Pine Rd",
		Notes:         "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = repo.IndexOrder(ctx, order)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Delete the order
	err = repo.DeleteOrder(ctx, order.ID)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Verify deletion
	_, err = repo.GetOrderByID(ctx, order.ID)
	assert.Error(t, err)
}

func TestElasticsearch_SearchOrders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	cfg := &config.ElasticsearchConfig{
		Addresses: []string{"http://localhost:9201"},
		IndexName: "test_orders_" + uuid.New().String()[:8],
	}

	client, err := elasticsearch.NewClient(cfg, logger)
	require.NoError(t, err)

	// Setup index with proper mappings
	err = setupTestIndex(ctx, client, cfg.IndexName)
	require.NoError(t, err)

	repo := elasticsearch.NewOrderRepository(client, cfg.IndexName)

	customerID := uuid.New().String()

	// Index multiple orders
	orders := []*model.Order{
		{
			ID:            uuid.New().String(),
			CustomerID:    customerID,
			CustomerName:  "Alice",
			CustomerEmail: "alice@example.com",
			ProductName:   "Wireless Headphones",
			ProductSKU:    "WH-001",
			Quantity:      1,
			Amount:        150.00,
			Currency:      "USD",
			Status:        model.StatusShipped,
			ShippingAddr:  "111 First St",
			Notes:         "",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
		{
			ID:            uuid.New().String(),
			CustomerID:    customerID,
			CustomerName:  "Alice",
			CustomerEmail: "alice@example.com",
			ProductName:   "Gaming Mouse",
			ProductSKU:    "MS-001",
			Quantity:      1,
			Amount:        75.00,
			Currency:      "USD",
			Status:        model.StatusDelivered,
			ShippingAddr:  "111 First St",
			Notes:         "",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}

	for _, order := range orders {
		err := repo.IndexOrder(ctx, order)
		require.NoError(t, err)
	}

	time.Sleep(2 * time.Second)

	// Search by status
	result, err := repo.SearchOrders(ctx, elasticsearch.SearchRequest{
		Status:   string(model.StatusShipped),
		Page:     1,
		PageSize: 10,
	})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, result.Total, int64(1))

	// Search by customer ID
	result, err = repo.SearchOrders(ctx, elasticsearch.SearchRequest{
		CustomerID: customerID,
		Page:       1,
		PageSize:   10,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
}
