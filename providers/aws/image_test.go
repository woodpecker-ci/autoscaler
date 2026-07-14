package aws

import (
	"bytes"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.woodpecker-ci.org/autoscaler/providers/aws/ec2api/mocks"
)

func TestValidateImageReference(t *testing.T) {
	t.Run("versioned alias", func(t *testing.T) {
		require.NoError(t, validateImageReference("debian-13"))
	})

	t.Run("broad alias is rejected", func(t *testing.T) {
		err := validateImageReference("debian")
		assert.ErrorContains(t, err, "unsupported aws-ami-id alias")
	})

	t.Run("alias region is rejected", func(t *testing.T) {
		err := validateImageReference("debian-13:us-east-1")
		assert.ErrorContains(t, err, "aws-ami-id must not specify a region")
	})

	t.Run("explicit image", func(t *testing.T) {
		require.NoError(t, validateImageReference("ami-1"))
	})

	t.Run("explicit image region is rejected", func(t *testing.T) {
		err := validateImageReference("ami-1:us-east-1")
		assert.ErrorContains(t, err, "aws-ami-id must not specify a region")
	})
}

func TestResolveVersionedImageAlias(t *testing.T) {
	var output bytes.Buffer
	originalLogger := log.Logger
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() {
		log.Logger = originalLogger
	})

	client := mocks.NewMockClient(t)
	client.On("DescribeImages", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeImagesInput) bool {
		return len(input.Owners) == 0 &&
			assert.ObjectsAreEqual([]string{"debian-13-amd64-*"}, filterValues(input.Filters, "name")) &&
			assert.ObjectsAreEqual([]string{"x86_64"}, filterValues(input.Filters, "architecture")) &&
			assert.ObjectsAreEqual([]string{"amazon"}, filterValues(input.Filters, "owner-alias")) &&
			assert.ObjectsAreEqual([]string{"ebs"}, filterValues(input.Filters, "root-device-type")) &&
			assert.ObjectsAreEqual([]string{"hvm"}, filterValues(input.Filters, "virtualization-type"))
	}), mock.MatchedBy(regionOptions("eu-central-1"))).Return(&ec2.DescribeImagesOutput{Images: []ec2_types.Image{
		verifiedDebianImage("ami-unverified-newest", "debian-13-amd64-20260714-1", "2026-07-14T00:00:00Z", ""),
		verifiedDebianImage("ami-marketplace", "debian-13-amd64-20260714-prod", "2026-07-14T00:00:00Z", "aws-marketplace"),
		verifiedDebianImage("ami-verified-old", "debian-13-amd64-20260712-1", "2026-07-12T00:00:00Z", "amazon"),
		verifiedDebianImage("ami-verified-new", "debian-13-amd64-20260713-1", "2026-07-13T00:00:00Z", "amazon"),
	}}, nil).Once()

	p := newTestProvider(client)
	image, err := p.resolveImage(t.Context(), "debian-13", "eu-central-1", ec2_types.ArchitectureValuesX8664)
	require.NoError(t, err)
	assert.Equal(t, "ami-verified-new", aws.ToString(image.ImageId))
	assert.JSONEq(t, `{
		"level":"info",
		"alias":"debian-13",
		"ami_id":"ami-verified-new",
		"region":"eu-central-1",
		"architecture":"x86_64",
		"message":"resolved AWS AMI alias"
	}`, output.String())
}

func verifiedDebianImage(id, name, created, ownerAlias string) ec2_types.Image {
	return ec2_types.Image{
		ImageId:            aws.String(id),
		Name:               aws.String(name),
		Architecture:       ec2_types.ArchitectureValuesX8664,
		CreationDate:       aws.String(created),
		ImageOwnerAlias:    aws.String(ownerAlias),
		Public:             aws.Bool(true),
		RootDeviceType:     ec2_types.DeviceTypeEbs,
		State:              ec2_types.ImageStateAvailable,
		VirtualizationType: ec2_types.VirtualizationTypeHvm,
	}
}

func filterValues(filters []ec2_types.Filter, name string) []string {
	for _, filter := range filters {
		if aws.ToString(filter.Name) == name {
			return filter.Values
		}
	}
	return nil
}
