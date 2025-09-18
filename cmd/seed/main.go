package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rohit/cdc-pipeline/internal/config"
	"github.com/rohit/cdc-pipeline/internal/db"
	"github.com/rohit/cdc-pipeline/internal/model"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx := context.Background()
	database, err := db.NewDB(ctx, &cfg.Postgres, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.Close()

	logger.Info("seeding database with sample orders")

	// Sample data
	customerIDs := []string{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	}

	customerNames := []string{
		"John Smith", "Jane Doe", "Bob Johnson", "Alice Williams", "Charlie Brown",
	}

	products := []struct {
		name string
		sku  string
		min  float64
		max  float64
	}{
		{"Wireless Headphones", "WH-1000X", 100, 350},
		{"Laptop Pro 15\"", "LT-PRO-15", 1200, 2500},
		{"Smartphone X", "SP-X-128", 500, 1200},
		{"Smart Watch Series 5", "SW-S5", 200, 600},
		{"4K Monitor 27\"", "MON-4K-27", 300, 800},
		{"Mechanical Keyboard", "KB-MECH-RGB", 80, 200},
		{"Gaming Mouse", "MS-GAME-PRO", 50, 150},
		{"USB-C Hub", "HUB-USBC-7", 30, 100},
		{"External SSD 1TB", "SSD-EXT-1TB", 100, 300},
		{"Wireless Charger", "CHG-WIRELESS", 20, 60},
		{"Running Shoes Pro", "SHOE-RUN-PRO", 80, 200},
		{"Yoga Mat Premium", "YOGA-MAT-P", 25, 80},
		{"Water Bottle 1L", "BTL-H2O-1L", 15, 40},
		{"Backpack Travel 40L", "BAG-TRV-40", 60, 150},
		{"Sunglasses Polarized", "SUN-POL-UV", 50, 200},
	}

	statuses := []model.OrderStatus{
		model.StatusPending,
		model.StatusConfirmed,
		model.StatusShipped,
		model.StatusDelivered,
		model.StatusCancelled,
	}

	addresses := []string{
		"123 Main St, New York, NY 10001",
		"456 Oak Ave, Los Angeles, CA 90001",
		"789 Pine Rd, Chicago, IL 60601",
		"321 Elm St, Houston, TX 77001",
		"654 Maple Dr, Phoenix, AZ 85001",
	}

	notes := []string{
		"Please deliver before 5 PM",
		"Leave at front door",
		"Gift wrap requested",
		"Urgent delivery needed",
		"",
		"",
		"",
	}

	rand.Seed(time.Now().UnixNano())

	// Create 50 orders
	for i := 0; i < 50; i++ {
		customerIdx := rand.Intn(len(customerIDs))
		productIdx := rand.Intn(len(products))
		product := products[productIdx]

		// Random date within last 6 months
		daysAgo := rand.Intn(180)
		_ = time.Now().AddDate(0, 0, -daysAgo) // createdAt - unused for now

		quantity := rand.Intn(3) + 1
		basePrice := product.min + rand.Float64()*(product.max-product.min)
		amount := basePrice * float64(quantity)

		order := &model.Order{
			ID:           uuid.New().String(),
			CustomerID:   customerIDs[customerIdx],
			CustomerName: customerNames[customerIdx],
			CustomerEmail: fmt.Sprintf("%s@example.com",
				[]string{"john.smith", "jane.doe", "bob.johnson", "alice.williams", "charlie.brown"}[customerIdx]),
			ProductName:  product.name,
			ProductSKU:   product.sku,
			Quantity:     quantity,
			Amount:       float64(int(amount*100)) / 100, // Round to 2 decimals
			Currency:     "USD",
			Status:       statuses[rand.Intn(len(statuses))],
			ShippingAddr: addresses[rand.Intn(len(addresses))],
			Notes:        notes[rand.Intn(len(notes))],
		}

		_, err := database.CreateOrder(ctx, order)
		if err != nil {
			logger.Error("failed to create order", zap.Error(err))
			continue
		}

		logger.Info("order created",
			zap.String("order_id", order.ID),
			zap.String("product", order.ProductName),
			zap.String("status", string(order.Status)),
		)

		// Small delay to avoid overwhelming the database
		time.Sleep(10 * time.Millisecond)
	}

	logger.Info("seeding completed", zap.Int("count", 50))
}
