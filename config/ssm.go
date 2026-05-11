// Package config loads application configuration from AWS SSM Parameter Store.
// When SSM_PREFIX is empty (local development), it is a no-op.
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmAPI is the slice of the AWS SSM client surface this package uses. Tests
// substitute a stub implementation; production callers transparently use the
// real ssm.Client.
type ssmAPI interface {
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
}

// ssmClientFactory builds an SSM client. Replaceable for testing.
var ssmClientFactory = func(ctx context.Context) (ssmAPI, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("config: load AWS config: %w", err)
	}
	return ssm.NewFromConfig(cfg), nil
}

// LoadFromSSM fetches all parameters under SSM_PREFIX and sets them as
// environment variables. Existing env vars are NOT overwritten, so .env
// files and Lambda env vars (GIN_MODE, SSM_PREFIX) take precedence.
//
// Returns immediately when SSM_PREFIX is unset or empty.
func LoadFromSSM(ctx context.Context) error {
	prefix := os.Getenv("SSM_PREFIX")
	if prefix == "" {
		return nil
	}

	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	client, err := ssmClientFactory(ctx)
	if err != nil {
		return err
	}

	var nextToken *string
	for {
		out, err := client.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
			Path:           aws.String(prefix),
			Recursive:      aws.Bool(false),
			WithDecryption: aws.Bool(true),
			NextToken:      nextToken,
		})
		if err != nil {
			return fmt.Errorf("config: GetParametersByPath(%s): %w", prefix, err)
		}

		for _, p := range out.Parameters {
			name := aws.ToString(p.Name)
			key := name[len(prefix):]
			if key == "" {
				continue
			}
			// Do not overwrite values already present in the environment.
			if os.Getenv(key) == "" {
				os.Setenv(key, aws.ToString(p.Value))
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return nil
}

// MustLoadFromSSM calls LoadFromSSM and panics on error.
func MustLoadFromSSM(ctx context.Context) {
	if err := LoadFromSSM(ctx); err != nil {
		panic(err)
	}
}
