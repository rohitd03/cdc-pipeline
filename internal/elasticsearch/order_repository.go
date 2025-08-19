package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/rohit/cdc-pipeline/internal/model"
	"go.uber.org/zap"
)

// OrderRepository defines operations for orders in Elasticsearch
type OrderRepository interface {
	IndexOrder(ctx context.Context, order *model.Order) error
	UpdateOrder(ctx context.Context, order *model.Order) error
	DeleteOrder(ctx context.Context, id string) error
	SearchOrders(ctx context.Context, req SearchRequest) (*SearchResult, error)
	GetOrderByID(ctx context.Context, id string) (*model.Order, error)
}

// orderRepository implements OrderRepository
type orderRepository struct {
	es        *elasticsearch.Client
	indexName string
	logger    *zap.Logger
}

// NewOrderRepository creates a new OrderRepository
func NewOrderRepository(client *Client, indexName string) OrderRepository {
	return &orderRepository{
		es:        client.GetClient(),
		indexName: indexName,
		logger:    client.logger,
	}
}

// SearchRequest contains search criteria for orders
type SearchRequest struct {
	Query      string
	Status     string
	CustomerID string
	MinAmount  float64
	MaxAmount  float64
	From       time.Time
	To         time.Time
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

// SearchResult contains search results
type SearchResult struct {
	Total    int64
	Orders   []*model.Order
	Page     int
	PageSize int
}

// IndexOrder indexes a new order into Elasticsearch
func (r *orderRepository) IndexOrder(ctx context.Context, order *model.Order) error {
	data, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to marshal order: %w", err)
	}

	res, err := r.es.Index(
		r.indexName,
		bytes.NewReader(data),
		r.es.Index.WithContext(ctx),
		r.es.Index.WithDocumentID(order.ID),
		r.es.Index.WithRefresh("true"),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to index order: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch: index operation failed: %s", res.String())
	}

	r.logger.Debug("order indexed",
		zap.String("order_id", order.ID),
		zap.String("index", r.indexName),
	)

	return nil
}

// UpdateOrder updates an existing order in Elasticsearch
func (r *orderRepository) UpdateOrder(ctx context.Context, order *model.Order) error {
	data, err := json.Marshal(map[string]interface{}{
		"doc": order,
	})
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to marshal order: %w", err)
	}

	res, err := r.es.Update(
		r.indexName,
		order.ID,
		bytes.NewReader(data),
		r.es.Update.WithContext(ctx),
		r.es.Update.WithRefresh("true"),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to update order: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		if res.StatusCode == 404 {
			// Document doesn't exist, index it instead
			return r.IndexOrder(ctx, order)
		}
		return fmt.Errorf("elasticsearch: update operation failed: %s", res.String())
	}

	r.logger.Debug("order updated",
		zap.String("order_id", order.ID),
		zap.String("index", r.indexName),
	)

	return nil
}

// DeleteOrder removes an order from Elasticsearch
func (r *orderRepository) DeleteOrder(ctx context.Context, id string) error {
	res, err := r.es.Delete(
		r.indexName,
		id,
		r.es.Delete.WithContext(ctx),
		r.es.Delete.WithRefresh("true"),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to delete order: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		if res.StatusCode == 404 {
			// Document doesn't exist, not an error for delete
			r.logger.Debug("order not found for deletion",
				zap.String("order_id", id),
			)
			return nil
		}
		return fmt.Errorf("elasticsearch: delete operation failed: %s", res.String())
	}

	r.logger.Debug("order deleted",
		zap.String("order_id", id),
		zap.String("index", r.indexName),
	)

	return nil
}

// SearchOrders searches for orders based on criteria
func (r *orderRepository) SearchOrders(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// Set defaults
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.SortBy == "" {
		req.SortBy = "created_at"
	}
	if req.SortOrder == "" {
		req.SortOrder = "desc"
	}

	// Build query
	query := r.buildQuery(req)

	// Calculate from offset
	from := (req.Page - 1) * req.PageSize

	var buf bytes.Buffer
	searchBody := map[string]interface{}{
		"query": query,
		"from":  from,
		"size":  req.PageSize,
		"sort": []map[string]interface{}{
			{
				req.SortBy: map[string]string{
					"order": req.SortOrder,
				},
			},
		},
	}

	if err := json.NewEncoder(&buf).Encode(searchBody); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to encode search query: %w", err)
	}

	res, err := r.es.Search(
		r.es.Search.WithContext(ctx),
		r.es.Search.WithIndex(r.indexName),
		r.es.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: search returned error: %s", res.String())
	}

	// Parse response
	var esResponse struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source model.Order `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &esResponse); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to parse response: %w", err)
	}

	orders := make([]*model.Order, 0, len(esResponse.Hits.Hits))
	for _, hit := range esResponse.Hits.Hits {
		order := hit.Source
		orders = append(orders, &order)
	}

	return &SearchResult{
		Total:    esResponse.Hits.Total.Value,
		Orders:   orders,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// GetOrderByID retrieves a single order by ID from Elasticsearch
func (r *orderRepository) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	res, err := r.es.Get(
		r.indexName,
		id,
		r.es.Get.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to get order: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		if res.StatusCode == 404 {
			return nil, fmt.Errorf("elasticsearch: order not found")
		}
		return nil, fmt.Errorf("elasticsearch: get operation failed: %s", res.String())
	}

	var esResponse struct {
		Source model.Order `json:"_source"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &esResponse); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to parse response: %w", err)
	}

	return &esResponse.Source, nil
}

// buildQuery constructs an Elasticsearch query based on search criteria
func (r *orderRepository) buildQuery(req SearchRequest) map[string]interface{} {
	must := make([]map[string]interface{}, 0)
	filter := make([]map[string]interface{}, 0)

	// Full-text search
	if req.Query != "" {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     req.Query,
				"fields":    []string{"product_name", "customer_name", "notes"},
				"fuzziness": "AUTO",
			},
		})
	}

	// Status filter
	if req.Status != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"status": req.Status,
			},
		})
	}

	// Customer ID filter
	if req.CustomerID != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"customer_id": req.CustomerID,
			},
		})
	}

	// Amount range filter
	if req.MinAmount > 0 || req.MaxAmount > 0 {
		rangeQuery := map[string]interface{}{}
		if req.MinAmount > 0 {
			rangeQuery["gte"] = req.MinAmount
		}
		if req.MaxAmount > 0 {
			rangeQuery["lte"] = req.MaxAmount
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{
				"amount": rangeQuery,
			},
		})
	}

	// Date range filter
	if !req.From.IsZero() || !req.To.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !req.From.IsZero() {
			rangeQuery["gte"] = req.From.Format(time.RFC3339)
		}
		if !req.To.IsZero() {
			rangeQuery["lte"] = req.To.Format(time.RFC3339)
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{
				"created_at": rangeQuery,
			},
		})
	}

	// Build bool query
	boolQuery := map[string]interface{}{}
	if len(must) > 0 {
		boolQuery["must"] = must
	}
	if len(filter) > 0 {
		boolQuery["filter"] = filter
	}

	// If no filters, match all
	if len(must) == 0 && len(filter) == 0 {
		return map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	}

	return map[string]interface{}{
		"bool": boolQuery,
	}
}
