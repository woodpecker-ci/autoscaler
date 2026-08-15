package aws

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type fakeIdentityClient struct {
	output *sts.GetCallerIdentityOutput
	err    error
}

func (f fakeIdentityClient) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f.output, f.err
}

func TestCredentialFlagsUseStandardAWSEnvironment(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	cmd := &cli.Command{
		Flags: ProviderFlags,
		Action: func(_ context.Context, cmd *cli.Command) error {
			assert.Equal(t, "test-access-key", cmd.String("aws-access-key-id"))
			assert.Equal(t, "test-secret-key", cmd.String("aws-secret-access-key"))
			return nil
		},
	}

	require.NoError(t, cmd.Run(t.Context(), []string{"autoscaler"}))
}

func TestStaticCredentialsOption(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		option, err := staticCredentialsOption("", "")
		require.NoError(t, err)
		assert.Nil(t, option)
	})

	t.Run("missing access key ID", func(t *testing.T) {
		_, err := staticCredentialsOption("", "secret")
		assert.ErrorContains(t, err, "aws-access-key-id")
	})

	t.Run("missing secret access key", func(t *testing.T) {
		_, err := staticCredentialsOption("access", "")
		assert.ErrorContains(t, err, "aws-secret-access-key")
	})

	t.Run("forwarded to SDK", func(t *testing.T) {
		option, err := staticCredentialsOption("test-access-key", "test-secret-key")
		require.NoError(t, err)
		require.NotNil(t, option)

		var options awsconfig.LoadOptions
		require.NoError(t, option(&options))
		require.NotNil(t, options.Credentials)

		credentials, err := options.Credentials.Retrieve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "test-access-key", credentials.AccessKeyID)
		assert.Equal(t, "test-secret-key", credentials.SecretAccessKey)
	})
}

func TestAuthenticateAWS(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var output bytes.Buffer
		originalLogger := log.Logger
		log.Logger = zerolog.New(&output)
		t.Cleanup(func() {
			log.Logger = originalLogger
		})

		err := authenticateAWS(t.Context(), fakeIdentityClient{output: &sts.GetCallerIdentityOutput{
			Account: aws.String("123456789012"),
			Arn:     aws.String("arn:aws:iam::123456789012:user/autoscaler"),
		}})
		require.NoError(t, err)
		assert.JSONEq(t, `{
			"level":"info",
			"account":"123456789012",
			"arn":"arn:aws:iam::123456789012:user/autoscaler",
			"message":"authenticated with AWS"
		}`, output.String())
	})

	t.Run("failure", func(t *testing.T) {
		err := authenticateAWS(t.Context(), fakeIdentityClient{err: errors.New("invalid credentials")})
		assert.ErrorContains(t, err, "authenticate with AWS: invalid credentials")
	})
}

func TestAuthenticationRegion(t *testing.T) {
	t.Run("default region", func(t *testing.T) {
		p := provider{name: "aws", region: "eu-central-1"}
		region, err := p.authenticationRegion(nil)
		require.NoError(t, err)
		assert.Equal(t, "eu-central-1", region)
	})

	t.Run("qualified instance type", func(t *testing.T) {
		p := provider{name: "aws"}
		region, err := p.authenticationRegion([]string{"t3.micro:us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", region)
	})

	t.Run("missing region", func(t *testing.T) {
		p := provider{name: "aws"}
		_, err := p.authenticationRegion([]string{"t3.micro"})
		assert.ErrorIs(t, err, ErrRegionNotSet)
	})
}
