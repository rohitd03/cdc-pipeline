package elasticsearch

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/rohit/cdc-pipeline/internal/config"
	"go.uber.org/zap"
)

// Client wraps the Elasticsearch client
type Client struct {
	es     *elasticsearch.Client
	logger *zap.Logger
}

// NewClient creates a new Elasticsearch client
func NewClient(cfg *config.ElasticsearchConfig, logger *zap.Logger) (*Client, error) {
	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	es, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	// Test the connection
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get elasticsearch info: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch returned error: %s", res.String())
	}

	logger.Info("elasticsearch connection established",
		zap.Strings("addresses", cfg.Addresses),
	)

	return &Client{
		es:     es,
		logger: logger,
	}, nil
}

// GetClient returns the underlying Elasticsearch client
func (c *Client) GetClient() *elasticsearch.Client {
	return c.es
}

// Ping checks if Elasticsearch is reachable
func (c *Client) Ping() error {
	res, err := c.es.Ping()
	if err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch ping returned error: %s", res.String())
	}

	return nil
}
