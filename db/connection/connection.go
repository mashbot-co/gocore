package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	instance *gorm.DB
	once     sync.Once
	initErr  error
)

// Setup builds the DSN from environment variables and initializes the database
// connection. This is the standard entry point for both APIs and CLI tools.
func Setup() (*gorm.DB, error) {
	dsn, err := BuildDSN()
	if err != nil {
		return nil, fmt.Errorf("build DSN: %w", err)
	}
	return Initialize(dsn)
}

// Initialize sets up the GORM database connection and registers all mixin
// callbacks via OnInitialize. Idempotent.
func Initialize(dsn string) (*gorm.DB, error) {
	once.Do(func() {
		instance, initErr = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger:                                   logger.Default.LogMode(logger.Warn),
			DisableForeignKeyConstraintWhenMigrating: true,
		})
		if initErr == nil {
			registerCallbacks(instance)
		}
	})
	return instance, initErr
}

// DB returns the initialized *gorm.DB instance. Panics if not initialized.
func DB() *gorm.DB {
	if instance == nil {
		panic("connection: not initialized — call Setup() or Initialize() first")
	}
	return instance
}

// Reset clears the singleton (for testing).
func Reset() {
	instance = nil
	initErr = nil
	once = sync.Once{}
}

// BuildDSN constructs a PostgreSQL DSN from environment variables.
// If DB_SECRET_ARN is set and not "local", credentials are fetched from AWS Secrets Manager.
func BuildDSN() (string, error) {
	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	dbname := envOrDefault("DB_NAME", "app")
	secretARN := os.Getenv("DB_SECRET_ARN")

	username, password, err := getCredentials(secretARN)
	if err != nil {
		return "", err
	}

	sslMode := "disable"
	if secretARN != "" && secretARN != "local" {
		sslMode = "require"
	}

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, username, password, dbname, sslMode), nil
}

type secretPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// secretFetcher is the function that retrieves a secret value. Replaceable for testing.
var secretFetcher = fetchSecretFromAWS

func getCredentials(secretARN string) (username, password string, err error) {
	if secretARN == "" || secretARN == "local" {
		return envOrDefault("DB_USERNAME", "postgres"), envOrDefault("DB_PASSWORD", "postgres"), nil
	}

	secretJSON, err := secretFetcher(secretARN)
	if err != nil {
		return "", "", err
	}

	var payload secretPayload
	if err := json.Unmarshal([]byte(secretJSON), &payload); err != nil {
		return "", "", fmt.Errorf("parse secret: %w", err)
	}

	return payload.Username, payload.Password, nil
}

func fetchSecretFromAWS(secretARN string) (string, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: &secretARN,
	})
	if err != nil {
		return "", fmt.Errorf("get secret: %w", err)
	}

	return *result.SecretString, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
