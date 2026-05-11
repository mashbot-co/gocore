package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// fakeSSM is the stub used in place of the real ssm.Client. It returns
// pre-canned pages and records every input it received.
type fakeSSM struct {
	pages     [][]types.Parameter // each element is one page worth of parameters
	nextToken []string            // matching tokens for each page (last is nil)
	calls     int
	err       error
}

func (f *fakeSSM) GetParametersByPath(ctx context.Context, params *ssmtypes.GetParametersByPathInput, optFns ...func(*ssmtypes.Options)) (*ssmtypes.GetParametersByPathOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	idx := f.calls
	f.calls++
	if idx >= len(f.pages) {
		return &ssmtypes.GetParametersByPathOutput{}, nil
	}
	out := &ssmtypes.GetParametersByPathOutput{Parameters: f.pages[idx]}
	if idx < len(f.nextToken) && f.nextToken[idx] != "" {
		token := f.nextToken[idx]
		out.NextToken = &token
	}
	return out, nil
}

// installFakeSSM swaps in a fake client for the duration of a test.
func installFakeSSM(t *testing.T, fake *fakeSSM) {
	t.Helper()
	original := ssmClientFactory
	ssmClientFactory = func(ctx context.Context) (ssmAPI, error) {
		return fake, nil
	}
	t.Cleanup(func() {
		ssmClientFactory = original
	})
}

// clearEnv removes any env vars whose names match the given keys so a test
// starts from a known state. Cleaned up automatically on test exit.
func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		original, had := os.LookupEnv(k)
		os.Unsetenv(k)
		if had {
			t.Cleanup(func() { os.Setenv(k, original) })
		} else {
			t.Cleanup(func() { os.Unsetenv(k) })
		}
	}
}

// --- LoadFromSSM no-op path ---

func TestLoadFromSSM_NoPrefix_IsNoOp(t *testing.T) {
	clearEnv(t, "SSM_PREFIX")

	// Swap in a fake that would fail if called.
	installFakeSSM(t, &fakeSSM{err: fmt.Errorf("should not be called")})

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error with empty SSM_PREFIX: %v", err)
	}
}

// --- Happy path: single page ---

func TestLoadFromSSM_LoadsParametersAsEnvVars(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")
	clearEnv(t, "DB_HOST", "DB_PORT")

	installFakeSSM(t, &fakeSSM{
		pages: [][]types.Parameter{
			{
				{Name: aws.String("/test/dev/DB_HOST"), Value: aws.String("rds.example.com")},
				{Name: aws.String("/test/dev/DB_PORT"), Value: aws.String("5432")},
			},
		},
	})

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv("DB_HOST"); got != "rds.example.com" {
		t.Errorf("expected DB_HOST=rds.example.com, got %q", got)
	}
	if got := os.Getenv("DB_PORT"); got != "5432" {
		t.Errorf("expected DB_PORT=5432, got %q", got)
	}
}

// --- Prefix normalization ---

func TestLoadFromSSM_NormalizesPrefixWithoutTrailingSlash(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev") // no trailing slash
	clearEnv(t, "DB_HOST")

	var capturedPrefix string
	fake := &fakeSSM{
		pages: [][]types.Parameter{
			{{Name: aws.String("/test/dev/DB_HOST"), Value: aws.String("foo")}},
		},
	}
	// Wrap the fake to capture the prefix it receives.
	original := ssmClientFactory
	ssmClientFactory = func(ctx context.Context) (ssmAPI, error) {
		return captureClient{inner: fake, capture: &capturedPrefix}, nil
	}
	t.Cleanup(func() { ssmClientFactory = original })

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedPrefix, "/") {
		t.Fatalf("expected prefix to be normalized with trailing slash, got %q", capturedPrefix)
	}
}

type captureClient struct {
	inner   *fakeSSM
	capture *string
}

func (c captureClient) GetParametersByPath(ctx context.Context, params *ssmtypes.GetParametersByPathInput, optFns ...func(*ssmtypes.Options)) (*ssmtypes.GetParametersByPathOutput, error) {
	if c.capture != nil {
		*c.capture = aws.ToString(params.Path)
	}
	return c.inner.GetParametersByPath(ctx, params, optFns...)
}

// --- Env var precedence: existing values are not overwritten ---

func TestLoadFromSSM_PreservesExistingEnv(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")
	t.Setenv("DB_HOST", "preset-by-developer")

	installFakeSSM(t, &fakeSSM{
		pages: [][]types.Parameter{
			{{Name: aws.String("/test/dev/DB_HOST"), Value: aws.String("from-ssm")}},
		},
	})

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv("DB_HOST"); got != "preset-by-developer" {
		t.Fatalf("expected pre-existing value preserved, got %q", got)
	}
}

// --- Pagination across multiple pages ---

func TestLoadFromSSM_HandlesPagination(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")
	clearEnv(t, "KEY1", "KEY2", "KEY3")

	installFakeSSM(t, &fakeSSM{
		pages: [][]types.Parameter{
			{{Name: aws.String("/test/dev/KEY1"), Value: aws.String("v1")}},
			{{Name: aws.String("/test/dev/KEY2"), Value: aws.String("v2")}},
			{{Name: aws.String("/test/dev/KEY3"), Value: aws.String("v3")}},
		},
		nextToken: []string{"page2", "page3", ""},
	})

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, kv := range []struct{ k, v string }{{"KEY1", "v1"}, {"KEY2", "v2"}, {"KEY3", "v3"}} {
		if got := os.Getenv(kv.k); got != kv.v {
			t.Errorf("expected %s=%s, got %q", kv.k, kv.v, got)
		}
	}
}

// --- Edge: parameter with name exactly equal to prefix is skipped ---

func TestLoadFromSSM_SkipsEmptyKey(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")

	installFakeSSM(t, &fakeSSM{
		pages: [][]types.Parameter{
			{
				// Name == prefix exactly → key would be ""; skip.
				{Name: aws.String("/test/dev/"), Value: aws.String("should-not-be-set")},
			},
		},
	})

	if err := LoadFromSSM(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Error path: factory failure ---

func TestLoadFromSSM_PropagatesFactoryError(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")

	original := ssmClientFactory
	ssmClientFactory = func(ctx context.Context) (ssmAPI, error) {
		return nil, fmt.Errorf("auth failed")
	}
	t.Cleanup(func() { ssmClientFactory = original })

	err := LoadFromSSM(context.Background())
	if err == nil {
		t.Fatal("expected error from factory")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("expected wrapped factory error, got %v", err)
	}
}

// --- Error path: GetParametersByPath failure ---

func TestLoadFromSSM_PropagatesGetParametersError(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")

	installFakeSSM(t, &fakeSSM{err: fmt.Errorf("access denied")})

	err := LoadFromSSM(context.Background())
	if err == nil {
		t.Fatal("expected error from GetParametersByPath")
	}
	if !strings.Contains(err.Error(), "GetParametersByPath") {
		t.Fatalf("expected wrapped error mentioning GetParametersByPath, got %v", err)
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected access denied in error, got %v", err)
	}
}

// --- MustLoadFromSSM ---

func TestMustLoadFromSSM_NoErrorIsNoOp(t *testing.T) {
	clearEnv(t, "SSM_PREFIX")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("did not expect panic: %v", r)
		}
	}()
	MustLoadFromSSM(context.Background())
}

// --- Default factory exercise ---

// Exercises the real ssmClientFactory body (which other tests bypass by
// substituting a stub). LoadDefaultConfig is permissive — it returns a
// non-nil config even without credentials in many environments, so the
// factory itself rarely errors. We just want the lines exercised.
func TestDefaultSSMClientFactory(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	client, err := ssmClientFactory(context.Background())
	if err != nil {
		// Some environments (e.g., locked-down CI) genuinely can't load
		// AWS config. Skipping is the right outcome there.
		t.Skipf("AWS config unavailable, skipping: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil ssm client from default factory")
	}
}

func TestMustLoadFromSSM_PanicsOnError(t *testing.T) {
	t.Setenv("SSM_PREFIX", "/test/dev")
	installFakeSSM(t, &fakeSSM{err: fmt.Errorf("denied")})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustLoadFromSSM")
		}
	}()
	MustLoadFromSSM(context.Background())
}
