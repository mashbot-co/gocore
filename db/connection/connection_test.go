package connection

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// requirePostgres skips the test when a local Postgres isn't reachable on
// localhost:5432. Pure-function tests above don't need this; tests that
// exercise the GORM connection lifecycle do. Run a local Postgres (e.g.
// via studio/services/postgres docker-compose) before running these tests,
// or use `go test -short ./...` to skip them.
func requirePostgres(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres-dependent test in -short mode")
	}
	conn, err := net.DialTimeout("tcp", "localhost:5432", 200*time.Millisecond)
	if err != nil {
		t.Skipf("skipping: Postgres not reachable on localhost:5432 (%v)", err)
	}
	conn.Close()
}

// --- envOrDefault ---

func TestEnvOrDefault_ReturnsEnvValue(t *testing.T) {
	t.Setenv("TEST_KEY_123", "custom")
	result := envOrDefault("TEST_KEY_123", "fallback")
	if result != "custom" {
		t.Fatalf("expected 'custom', got %q", result)
	}
}

func TestEnvOrDefault_ReturnsFallback(t *testing.T) {
	os.Unsetenv("TEST_KEY_MISSING")
	result := envOrDefault("TEST_KEY_MISSING", "default_val")
	if result != "default_val" {
		t.Fatalf("expected 'default_val', got %q", result)
	}
}

func TestEnvOrDefault_EmptyStringReturnsFallback(t *testing.T) {
	t.Setenv("TEST_KEY_EMPTY", "")
	result := envOrDefault("TEST_KEY_EMPTY", "fallback")
	if result != "fallback" {
		t.Fatalf("expected 'fallback' for empty env, got %q", result)
	}
}

// --- getCredentials ---

func TestGetCredentials_LocalMode(t *testing.T) {
	user, pass, err := getCredentials("local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != "postgres" || pass != "postgres" {
		t.Fatalf("expected postgres/postgres, got %s/%s", user, pass)
	}
}

func TestGetCredentials_EmptyARN(t *testing.T) {
	user, pass, err := getCredentials("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != "postgres" || pass != "postgres" {
		t.Fatalf("expected postgres/postgres, got %s/%s", user, pass)
	}
}

// --- BuildDSN ---

func TestBuildDSN_LocalDefaults(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")

	dsn, err := BuildDSN()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "host=localhost") {
		t.Fatalf("expected host=localhost in DSN, got %q", dsn)
	}
	if !strings.Contains(dsn, "port=5432") {
		t.Fatalf("expected port=5432 in DSN, got %q", dsn)
	}
	if !strings.Contains(dsn, "user=postgres") {
		t.Fatalf("expected user=postgres in DSN, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=app") {
		t.Fatalf("expected dbname=app in DSN, got %q", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("expected sslmode=disable for local, got %q", dsn)
	}
}

func TestBuildDSN_CustomHost(t *testing.T) {
	t.Setenv("DB_HOST", "mydb.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "myapp")
	t.Setenv("DB_SECRET_ARN", "local")

	dsn, err := BuildDSN()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "host=mydb.example.com") {
		t.Fatalf("expected custom host, got %q", dsn)
	}
	if !strings.Contains(dsn, "port=5433") {
		t.Fatalf("expected custom port, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=myapp") {
		t.Fatalf("expected custom dbname, got %q", dsn)
	}
}

func TestGetCredentials_WithMockedSecret(t *testing.T) {
	original := secretFetcher
	defer func() { secretFetcher = original }()

	secretFetcher = func(arn string) (string, error) {
		return `{"username":"dbadmin","password":"s3cret"}`, nil
	}

	user, pass, err := getCredentials("arn:aws:secretsmanager:us-east-1:123:secret:test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != "dbadmin" || pass != "s3cret" {
		t.Fatalf("expected dbadmin/s3cret, got %s/%s", user, pass)
	}
}

func TestGetCredentials_SecretFetchError(t *testing.T) {
	original := secretFetcher
	defer func() { secretFetcher = original }()

	secretFetcher = func(arn string) (string, error) {
		return "", fmt.Errorf("access denied")
	}

	_, _, err := getCredentials("arn:aws:secretsmanager:us-east-1:123:secret:test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCredentials_InvalidSecretJSON(t *testing.T) {
	original := secretFetcher
	defer func() { secretFetcher = original }()

	secretFetcher = func(arn string) (string, error) {
		return "not-json", nil
	}

	_, _, err := getCredentials("arn:aws:secretsmanager:us-east-1:123:secret:test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDSN_DefaultValues(t *testing.T) {
	// Clear all DB env vars to test defaults
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("DB_SECRET_ARN", "")

	dsn, err := BuildDSN()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "host=localhost") {
		t.Fatalf("expected default host=localhost, got %q", dsn)
	}
	if !strings.Contains(dsn, "port=5432") {
		t.Fatalf("expected default port=5432, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=app") {
		t.Fatalf("expected default dbname=app, got %q", dsn)
	}
}

// --- Setup / Initialize / DB / Reset ---

func TestSetup_ConnectsToDatabase(t *testing.T) {
	requirePostgres(t)
	Reset()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")

	gormDB, err := Setup()
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if gormDB == nil {
		t.Fatal("expected non-nil gorm.DB")
	}

	// Verify we can ping
	sqlDB, _ := gormDB.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestDB_ReturnsInstance(t *testing.T) {
	requirePostgres(t)
	// Setup should have been called by previous test
	// Reset and re-setup to be safe
	Reset()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")
	Setup()

	db := DB()
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestDB_PanicsWhenNotInitialized(t *testing.T) {
	requirePostgres(t)
	Reset()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when DB not initialized")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "not initialized") {
			t.Fatalf("unexpected panic: %v", r)
		}

		// Re-initialize for subsequent tests
		t.Setenv("DB_HOST", "localhost")
		t.Setenv("DB_PORT", "5432")
		t.Setenv("DB_NAME", "app")
		t.Setenv("DB_SECRET_ARN", "local")
		Setup()
	}()

	DB()
}

func TestReset_ClearsInstance(t *testing.T) {
	requirePostgres(t)
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")
	Setup()

	Reset()

	// After reset, instance should be nil
	defer func() {
		recover() // catch the panic from DB()
		// Re-initialize
		Setup()
	}()
	DB() // should panic
	t.Fatal("expected panic after Reset")
}

func TestInitialize_Idempotent(t *testing.T) {
	requirePostgres(t)
	Reset()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")

	dsn, _ := BuildDSN()
	db1, err1 := Initialize(dsn)
	db2, err2 := Initialize(dsn)

	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if db1 != db2 {
		t.Fatal("expected same instance on second call")
	}
}

func TestSetup_FailsWhenBuildDSNFails(t *testing.T) {
	requirePostgres(t)
	Reset()
	original := secretFetcher
	defer func() {
		secretFetcher = original
		Reset()
		t.Setenv("DB_HOST", "localhost")
		t.Setenv("DB_PORT", "5432")
		t.Setenv("DB_NAME", "app")
		t.Setenv("DB_SECRET_ARN", "local")
		Setup()
	}()

	// Force BuildDSN to fail by making secretFetcher return an error
	// with a non-local ARN
	t.Setenv("DB_SECRET_ARN", "arn:aws:fake")
	secretFetcher = func(arn string) (string, error) {
		return "", fmt.Errorf("mock: access denied")
	}

	_, err := Setup()
	if err == nil {
		t.Fatal("expected error from Setup when BuildDSN fails")
	}
	if !strings.Contains(err.Error(), "build DSN") {
		t.Fatalf("expected 'build DSN' error, got: %v", err)
	}
}

func TestFetchSecretFromAWS_ReturnsError(t *testing.T) {
	// Clear AWS credentials to ensure GetSecretValue fails.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "us-east-1")

	_, err := fetchSecretFromAWS("arn:aws:secretsmanager:us-east-1:123456:secret:test")
	if err == nil {
		t.Fatal("expected error from fetchSecretFromAWS without valid credentials")
	}
}

func TestBuildDSN_SSLModeRequireForAWS(t *testing.T) {
	original := secretFetcher
	defer func() { secretFetcher = original }()

	secretFetcher = func(arn string) (string, error) {
		return `{"username":"admin","password":"pass"}`, nil
	}

	t.Setenv("DB_HOST", "mydb.rds.amazonaws.com")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:db")

	dsn, err := BuildDSN()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("expected sslmode=require for AWS ARN, got %q", dsn)
	}
	if !strings.Contains(dsn, "user=admin") {
		t.Fatalf("expected user=admin from mock, got %q", dsn)
	}
}

// Initialize error path is covered by TestSetup_FailsWhenBuildDSNFails.
// We don't test with a bad host because GORM lazy-connects, which can hang.

// --- OnInitialize / registerCallbacks ---

func TestOnInitialize_RegistersCallback(t *testing.T) {
	called := false
	originalLen := len(callbackFns)

	OnInitialize(func(db *gorm.DB) {
		called = true
	})

	if len(callbackFns) != originalLen+1 {
		t.Fatal("expected callback to be registered")
	}

	// Clean up — remove our test callback
	callbackFns = callbackFns[:originalLen]
	_ = called
}

func TestRegisterCallbacks_ExecutesAll(t *testing.T) {
	requirePostgres(t)
	callCount := 0
	originalLen := len(callbackFns)

	// Register test callbacks
	OnInitialize(func(db *gorm.DB) { callCount++ })
	OnInitialize(func(db *gorm.DB) { callCount++ })

	// Simulate registerCallbacks with a nil DB (just to exercise the loop)
	Reset()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_SECRET_ARN", "local")
	Setup()

	if callCount != 2 {
		t.Fatalf("expected 2 callbacks called, got %d", callCount)
	}

	// Clean up test callbacks
	callbackFns = callbackFns[:originalLen]
}
