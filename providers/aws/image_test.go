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
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/providers/aws/ec2api/mocks"
)

func TestValidateImageReference(t *testing.T) {
	t.Run("aliases", func(t *testing.T) {
		for _, alias := range []string{
			"ubuntu-26.04-server",
			"ubuntu-20.04-server",
			"amazon-linux",
			"amazon_linux",
			"amazon-linux-2023",
			"suse",
			"suse-15",
			"suse-16",
			"debian-13",
		} {
			require.NoError(t, validateImageReference(alias), alias)
		}
	})

	t.Run("broad alias is rejected", func(t *testing.T) {
		for _, alias := range []string{"ubuntu", "ubuntu-server", "debian", "suse-", "amazon-linux-"} {
			err := validateImageReference(alias)
			assert.ErrorContains(t, err, "unsupported aws-ami-id alias", alias)
		}
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

func TestDefaultImageReferenceIsValid(t *testing.T) {
	for _, flag := range ProviderFlags {
		imageFlag, ok := flag.(*cli.StringFlag)
		if !ok || imageFlag.Name != "aws-ami-id" {
			continue
		}
		require.NoError(t, validateImageReference(imageFlag.Value))
		return
	}
	t.Fatal("aws-ami-id flag not found")
}

func TestResolveImageAliases(t *testing.T) {
	tests := []struct {
		name         string
		alias        string
		nameFilter   string
		selectedName string
		rejectedName string
	}{
		{
			name:         "Ubuntu",
			alias:        "ubuntu-26.04-server",
			nameFilter:   "ubuntu/images/hvm-ssd*/ubuntu-*-26.04-amd64-server-*",
			selectedName: "ubuntu/images/hvm-ssd-gp3/ubuntu-resolute-26.04-amd64-server-20260714",
			rejectedName: "ubuntu-pro-server/images/hvm-ssd-gp3/ubuntu-resolute-26.04-amd64-pro-server-20260714",
		},
		{
			// releases before 22.04 are published under hvm-ssd instead of hvm-ssd-gp3
			name:         "UbuntuOldStorageGeneration",
			alias:        "ubuntu-20.04-server",
			nameFilter:   "ubuntu/images/hvm-ssd*/ubuntu-*-20.04-amd64-server-*",
			selectedName: "ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-20250624",
			rejectedName: "ubuntu-minimal/images/hvm-ssd/ubuntu-focal-20.04-amd64-minimal-server-20250624",
		},
		{
			name:         "AmazonLinux",
			alias:        "amazon_linux",
			nameFilter:   "al2023-ami-2023.*-kernel-*-x86_64",
			selectedName: "al2023-ami-2023.12.20260710.0-kernel-6.18-x86_64",
			rejectedName: "al2023-ami-minimal-2023.12.20260710.0-kernel-6.18-x86_64",
		},
		{
			name:         "AmazonLinuxVersioned",
			alias:        "amazon-linux-2023",
			nameFilter:   "al2023-ami-2023.*-kernel-*-x86_64",
			selectedName: "al2023-ami-2023.12.20260710.0-kernel-6.18-x86_64",
			rejectedName: "al2023-ami-minimal-2023.12.20260710.0-kernel-6.18-x86_64",
		},
		{
			name:         "SUSE",
			alias:        "suse",
			nameFilter:   "suse-sles-16-*-hvm-ssd-x86_64",
			selectedName: "suse-sles-16-0-v20260703-hvm-ssd-x86_64",
			rejectedName: "suse-sles-16-0-v20260703-ecs-hvm-ssd-x86_64",
		},
		{
			// 15.x releases are published with service pack naming (spN)
			name:         "SUSEServicePackNaming",
			alias:        "suse-15",
			nameFilter:   "suse-sles-15-*-hvm-ssd-x86_64",
			selectedName: "suse-sles-15-sp7-v20260630-hvm-ssd-x86_64",
			rejectedName: "suse-sles-15-sp4-sapcal-v20260703-hvm-ssd-x86_64",
		},
		{
			name:         "Debian",
			alias:        "debian-13",
			nameFilter:   "debian-13-amd64-*",
			selectedName: "debian-13-amd64-20260712-2537",
			rejectedName: "debian-13-backports-amd64-20260712-2537",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := mocks.NewMockClient(t)
			client.On("DescribeImages", mock.Anything, mock.MatchedBy(func(input *ec2.DescribeImagesInput) bool {
				return len(input.Owners) == 0 &&
					assert.ObjectsAreEqual([]string{test.nameFilter}, filterValues(input.Filters, "name")) &&
					assert.ObjectsAreEqual([]string{"x86_64"}, filterValues(input.Filters, "architecture")) &&
					assert.ObjectsAreEqual([]string{"amazon"}, filterValues(input.Filters, "owner-alias")) &&
					assert.ObjectsAreEqual([]string{"ebs"}, filterValues(input.Filters, "root-device-type")) &&
					assert.ObjectsAreEqual([]string{"hvm"}, filterValues(input.Filters, "virtualization-type"))
			}), mock.MatchedBy(regionOptions("eu-central-1"))).Return(&ec2.DescribeImagesOutput{Images: []ec2_types.Image{
				verifiedImage("ami-rejected-newest", test.rejectedName, "2026-07-15T00:00:00Z", "amazon"),
				verifiedImage("ami-unverified", test.selectedName, "2026-07-15T00:00:00Z", ""),
				verifiedImage("ami-selected", test.selectedName, "2026-07-14T00:00:00Z", "amazon"),
				verifiedImage("ami-old", test.selectedName, "2026-07-13T00:00:00Z", "amazon"),
			}}, nil).Once()

			p := newTestProvider(client)
			image, err := p.resolveImage(t.Context(), test.alias, "eu-central-1", ec2_types.ArchitectureValuesX8664)
			require.NoError(t, err)
			assert.Equal(t, "ami-selected", aws.ToString(image.ImageId))
		})
	}
}

func TestResolveImageAliasLogsAMI(t *testing.T) {
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
		verifiedImage("ami-unverified-newest", "debian-13-amd64-20260714-1", "2026-07-14T00:00:00Z", ""),
		verifiedImage("ami-marketplace", "debian-13-amd64-20260714-prod", "2026-07-14T00:00:00Z", "aws-marketplace"),
		verifiedImage("ami-verified-old", "debian-13-amd64-20260712-1", "2026-07-12T00:00:00Z", "amazon"),
		verifiedImage("ami-verified-new", "debian-13-amd64-20260713-1", "2026-07-13T00:00:00Z", "amazon"),
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

func verifiedImage(id, name, created, ownerAlias string) ec2_types.Image {
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
