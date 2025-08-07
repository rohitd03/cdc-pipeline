package model

import "time"

// OrderStatus represents the current status of an order
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusShipped   OrderStatus = "shipped"
	StatusDelivered OrderStatus = "delivered"
	StatusCancelled OrderStatus = "cancelled"
)

// Order represents an order in the system
type Order struct {
	ID            string      `json:"id" db:"id"`
	CustomerID    string      `json:"customer_id" db:"customer_id"`
	CustomerName  string      `json:"customer_name" db:"customer_name"`
	CustomerEmail string      `json:"customer_email" db:"customer_email"`
	ProductName   string      `json:"product_name" db:"product_name"`
	ProductSKU    string      `json:"product_sku" db:"product_sku"`
	Quantity      int         `json:"quantity" db:"quantity"`
	Amount        float64     `json:"amount" db:"amount"`
	Currency      string      `json:"currency" db:"currency"`
	Status        OrderStatus `json:"status" db:"status"`
	ShippingAddr  string      `json:"shipping_address" db:"shipping_address"`
	Notes         string      `json:"notes" db:"notes"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" db:"updated_at"`
}

// DebeziumEvent represents the raw event received from Kafka/Debezium
type DebeziumEvent struct {
	Schema  interface{}     `json:"schema"`
	Payload DebeziumPayload `json:"payload"`
}

// DebeziumPayload contains the before/after state and metadata
type DebeziumPayload struct {
	Before      *OrderDBRow    `json:"before"` // nil for INSERT
	After       *OrderDBRow    `json:"after"`  // nil for DELETE
	Source      DebeziumSource `json:"source"`
	Op          string         `json:"op"` // c=create, u=update, d=delete, r=read(snapshot)
	TsMs        int64          `json:"ts_ms"`
	Transaction *interface{}   `json:"transaction"`
}

// DebeziumSource contains metadata about the source of the change
type DebeziumSource struct {
	Version   string `json:"version"`
	Connector string `json:"connector"`
	Name      string `json:"name"`
	TsMs      int64  `json:"ts_ms"`
	Snapshot  string `json:"snapshot"`
	DB        string `json:"db"`
	Schema    string `json:"schema"`
	Table     string `json:"table"`
	TxID      int64  `json:"txId"`
	LSN       int64  `json:"lsn"`
}

// OrderDBRow is how Debezium represents a row from Postgres
type OrderDBRow struct {
	ID            string  `json:"id"`
	CustomerID    string  `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	CustomerEmail string  `json:"customer_email"`
	ProductName   string  `json:"product_name"`
	ProductSKU    string  `json:"product_sku"`
	Quantity      int     `json:"quantity"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Status        string  `json:"status"`
	ShippingAddr  string  `json:"shipping_address"`
	Notes         string  `json:"notes"`
	CreatedAt     int64   `json:"created_at"` // Debezium sends timestamps as microseconds epoch
	UpdatedAt     int64   `json:"updated_at"`
}

// ToOrder converts OrderDBRow to Order
func (r *OrderDBRow) ToOrder() *Order {
	return &Order{
		ID:            r.ID,
		CustomerID:    r.CustomerID,
		CustomerName:  r.CustomerName,
		CustomerEmail: r.CustomerEmail,
		ProductName:   r.ProductName,
		ProductSKU:    r.ProductSKU,
		Quantity:      r.Quantity,
		Amount:        r.Amount,
		Currency:      r.Currency,
		Status:        OrderStatus(r.Status),
		ShippingAddr:  r.ShippingAddr,
		Notes:         r.Notes,
		CreatedAt:     time.UnixMicro(r.CreatedAt),
		UpdatedAt:     time.UnixMicro(r.UpdatedAt),
	}
}
