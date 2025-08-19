package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/model"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DBInterface defines the database operations
type DBInterface interface {
	CreateOrder(ctx context.Context, order *model.Order) (*model.Order, error)
	UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error)
	GetOrderByID(ctx context.Context, id string) (*model.Order, error)
	ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error)
	DeleteOrder(ctx context.Context, id string) error
	Ping(ctx context.Context) error
	Close()
}

// DB wraps the PostgreSQL connection pool
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewDB creates a new database connection pool
func NewDB(ctx context.Context, cfg *config.PostgresConfig, logger *zap.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxConns)
	poolConfig.MinConns = int32(cfg.MinConns)
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connection established",
		zap.String("host", cfg.Host),
		zap.String("database", cfg.DBName),
	)

	return &DB{
		pool:   pool,
		logger: logger,
	}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.pool.Close()
	db.logger.Info("database connection closed")
}

// Ping checks if the database is reachable
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// RunMigrations applies all SQL migrations from the migrations directory
func (db *DB) RunMigrations(ctx context.Context) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Sort migration files to ensure they run in order
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, file := range files {
		db.logger.Info("running migration", zap.String("file", file))

		content, err := migrationsFS.ReadFile("migrations/" + file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		if _, err := db.pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", file, err)
		}

		db.logger.Info("migration completed", zap.String("file", file))
	}

	return nil
}

// CreateOrder inserts a new order into the database
func (db *DB) CreateOrder(ctx context.Context, order *model.Order) (*model.Order, error) {
	query := `
		INSERT INTO orders (
			id, customer_id, customer_name, customer_email,
			product_name, product_sku, quantity, amount, currency,
			status, shipping_address, notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
		RETURNING id, customer_id, customer_name, customer_email,
			product_name, product_sku, quantity, amount, currency,
			status, shipping_address, notes, created_at, updated_at
	`

	var result model.Order
	err := db.pool.QueryRow(ctx, query,
		order.ID,
		order.CustomerID,
		order.CustomerName,
		order.CustomerEmail,
		order.ProductName,
		order.ProductSKU,
		order.Quantity,
		order.Amount,
		order.Currency,
		order.Status,
		order.ShippingAddr,
		order.Notes,
	).Scan(
		&result.ID,
		&result.CustomerID,
		&result.CustomerName,
		&result.CustomerEmail,
		&result.ProductName,
		&result.ProductSKU,
		&result.Quantity,
		&result.Amount,
		&result.Currency,
		&result.Status,
		&result.ShippingAddr,
		&result.Notes,
		&result.CreatedAt,
		&result.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create order: %w", err)
	}

	return &result, nil
}

// UpdateOrderStatus updates the status of an existing order
func (db *DB) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	query := `
		UPDATE orders
		SET status = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING id, customer_id, customer_name, customer_email,
			product_name, product_sku, quantity, amount, currency,
			status, shipping_address, notes, created_at, updated_at
	`

	var result model.Order
	err := db.pool.QueryRow(ctx, query, status, id).Scan(
		&result.ID,
		&result.CustomerID,
		&result.CustomerName,
		&result.CustomerEmail,
		&result.ProductName,
		&result.ProductSKU,
		&result.Quantity,
		&result.Amount,
		&result.Currency,
		&result.Status,
		&result.ShippingAddr,
		&result.Notes,
		&result.CreatedAt,
		&result.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("postgres: %w", fmt.Errorf("order with id %s not found", id))
		}
		return nil, fmt.Errorf("postgres: failed to update order status: %w", err)
	}

	return &result, nil
}

// GetOrderByID retrieves an order by its ID
func (db *DB) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	query := `
		SELECT id, customer_id, customer_name, customer_email,
			product_name, product_sku, quantity, amount, currency,
			status, shipping_address, notes, created_at, updated_at
		FROM orders
		WHERE id = $1
	`

	var result model.Order
	err := db.pool.QueryRow(ctx, query, id).Scan(
		&result.ID,
		&result.CustomerID,
		&result.CustomerName,
		&result.CustomerEmail,
		&result.ProductName,
		&result.ProductSKU,
		&result.Quantity,
		&result.Amount,
		&result.Currency,
		&result.Status,
		&result.ShippingAddr,
		&result.Notes,
		&result.CreatedAt,
		&result.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("postgres: %w", fmt.Errorf("order with id %s not found", id))
		}
		return nil, fmt.Errorf("postgres: failed to get order: %w", err)
	}

	return &result, nil
}

// ListOrders retrieves a paginated list of orders
func (db *DB) ListOrders(ctx context.Context, limit, offset int) ([]*model.Order, error) {
	query := `
		SELECT id, customer_id, customer_name, customer_email,
			product_name, product_sku, quantity, amount, currency,
			status, shipping_address, notes, created_at, updated_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := db.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to list orders: %w", err)
	}
	defer rows.Close()

	var orders []*model.Order
	for rows.Next() {
		var order model.Order
		err := rows.Scan(
			&order.ID,
			&order.CustomerID,
			&order.CustomerName,
			&order.CustomerEmail,
			&order.ProductName,
			&order.ProductSKU,
			&order.Quantity,
			&order.Amount,
			&order.Currency,
			&order.Status,
			&order.ShippingAddr,
			&order.Notes,
			&order.CreatedAt,
			&order.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: failed to scan order: %w", err)
		}
		orders = append(orders, &order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: error iterating orders: %w", err)
	}

	return orders, nil
}

// DeleteOrder deletes an order by its ID
func (db *DB) DeleteOrder(ctx context.Context, id string) error {
	query := `DELETE FROM orders WHERE id = $1`

	result, err := db.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("postgres: failed to delete order: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("postgres: %w", fmt.Errorf("order with id %s not found", id))
	}

	return nil
}
