package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server        ServerConfig
	Postgres      PostgresConfig
	Kafka         KafkaConfig
	Elasticsearch ElasticsearchConfig
	Log           LogConfig
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// PostgresConfig contains PostgreSQL connection settings
type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int
	MinConns int
}

// KafkaConfig contains Kafka connection and consumer settings
type KafkaConfig struct {
	Brokers     []string
	GroupID     string
	Topic       string
	DLQTopic    string
	MaxRetries  int
	RetryBaseMs int
}

// ElasticsearchConfig contains Elasticsearch connection settings
type ElasticsearchConfig struct {
	Addresses []string
	Username  string
	Password  string
	IndexName string
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// NewConfig loads configuration from environment variables
func NewConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", ""),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnv("POSTGRES_USER", ""),
			Password: getEnv("POSTGRES_PASSWORD", ""),
			DBName:   getEnv("POSTGRES_DB", ""),
			SSLMode:  getEnv("POSTGRES_SSL_MODE", "disable"),
			MaxConns: getIntEnv("POSTGRES_MAX_CONNS", 10),
			MinConns: getIntEnv("POSTGRES_MIN_CONNS", 2),
		},
		Kafka: KafkaConfig{
			Brokers:     getSliceEnv("KAFKA_BROKERS", []string{"localhost:9092"}),
			GroupID:     getEnv("KAFKA_GROUP_ID", "cdc-pipeline-consumer"),
			Topic:       getEnv("KAFKA_TOPIC", "postgres.public.orders"),
			DLQTopic:    getEnv("KAFKA_DLQ_TOPIC", "postgres.public.orders.dlq"),
			MaxRetries:  getIntEnv("KAFKA_MAX_RETRIES", 3),
			RetryBaseMs: getIntEnv("KAFKA_RETRY_BASE_MS", 100),
		},
		Elasticsearch: ElasticsearchConfig{
			Addresses: getSliceEnv("ES_ADDRESSES", []string{"http://localhost:9200"}),
			Username:  getEnv("ES_USERNAME", ""),
			Password:  getEnv("ES_PASSWORD", ""),
			IndexName: getEnv("ES_INDEX_NAME", "orders"),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	// Validate required fields
	if cfg.Postgres.Host == "" {
		return nil, fmt.Errorf("POSTGRES_HOST is required")
	}
	if cfg.Postgres.User == "" {
		return nil, fmt.Errorf("POSTGRES_USER is required")
	}
	if cfg.Postgres.DBName == "" {
		return nil, fmt.Errorf("POSTGRES_DB is required")
	}

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnv retrieves an integer environment variable or returns a default value
func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getDurationEnv retrieves a duration environment variable or returns a default value
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getSliceEnv retrieves a comma-separated environment variable as a slice
func getSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// ConnectionString returns the PostgreSQL connection string
func (p *PostgresConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.SSLMode,
	)
}
