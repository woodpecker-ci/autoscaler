package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/rs/zerolog/log"
)

func validateImageReference(reference string) error {
	if reference == "" {
		return ErrAMINotFound
	}
	if strings.Contains(reference, ":") {
		return fmt.Errorf("aws-ami-id must not specify a region: %s", reference)
	}
	if isImageAlias(reference) || strings.HasPrefix(reference, "ami-") {
		return nil
	}
	return fmt.Errorf("unsupported aws-ami-id alias: %s", reference)
}

func isImageAlias(reference string) bool {
	version, found := strings.CutPrefix(reference, "debian-")
	if !found || version == "" {
		return false
	}
	for _, char := range version {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (p *provider) resolveImage(ctx context.Context, reference, region string, architecture ec2_types.ArchitectureValues) (ec2_types.Image, error) {
	if isImageAlias(reference) {
		return p.resolveDebianImage(ctx, reference, region, architecture)
	}

	images, err := p.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{reference},
	}, regionOpt(region))
	if err != nil {
		return ec2_types.Image{}, fmt.Errorf("%s: DescribeImages %q in %q: %w", p.name, reference, region, err)
	}
	if len(images.Images) != 1 {
		return ec2_types.Image{}, fmt.Errorf("%s: %w: %s in %q", p.name, ErrAMINotFound, reference, region)
	}
	return images.Images[0], nil
}

func (p *provider) resolveDebianImage(ctx context.Context, alias, region string, architecture ec2_types.ArchitectureValues) (ec2_types.Image, error) {
	nameArchitecture, err := debianArchitecture(architecture)
	if err != nil {
		return ec2_types.Image{}, fmt.Errorf("%s: %w", p.name, err)
	}
	namePrefix := alias + "-" + nameArchitecture + "-"
	images, err := p.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []ec2_types.Filter{
			{Name: aws.String("architecture"), Values: []string{string(architecture)}},
			{Name: aws.String("is-public"), Values: []string{"true"}},
			{Name: aws.String("name"), Values: []string{namePrefix + "*"}},
			{Name: aws.String("owner-alias"), Values: []string{"amazon"}},
			{Name: aws.String("root-device-type"), Values: []string{string(ec2_types.DeviceTypeEbs)}},
			{Name: aws.String("state"), Values: []string{string(ec2_types.ImageStateAvailable)}},
			{Name: aws.String("virtualization-type"), Values: []string{string(ec2_types.VirtualizationTypeHvm)}},
		},
	}, regionOpt(region))
	if err != nil {
		return ec2_types.Image{}, fmt.Errorf("%s: DescribeImages alias %q in %q: %w", p.name, alias, region, err)
	}

	var newest ec2_types.Image
	for _, image := range images.Images {
		if !isVerifiedDebianImage(image, namePrefix, architecture) {
			continue
		}
		if newest.ImageId == nil || newerImage(image, newest) {
			newest = image
		}
	}
	if newest.ImageId == nil {
		return ec2_types.Image{}, fmt.Errorf("%s: %w: verified provider alias %q for %s in %q",
			p.name, ErrAMINotFound, alias, architecture, region)
	}
	log.Info().
		Str("alias", alias).
		Str("ami_id", aws.ToString(newest.ImageId)).
		Str("region", region).
		Str("architecture", string(architecture)).
		Msg("resolved AWS AMI alias")
	return newest, nil
}

func isVerifiedDebianImage(image ec2_types.Image, namePrefix string, architecture ec2_types.ArchitectureValues) bool {
	return image.Architecture == architecture &&
		strings.HasPrefix(aws.ToString(image.Name), namePrefix) &&
		aws.ToString(image.ImageOwnerAlias) == "amazon" &&
		aws.ToBool(image.Public) &&
		image.RootDeviceType == ec2_types.DeviceTypeEbs &&
		image.State == ec2_types.ImageStateAvailable &&
		image.VirtualizationType == ec2_types.VirtualizationTypeHvm
}

func newerImage(left, right ec2_types.Image) bool {
	leftCreationDate := aws.ToString(left.CreationDate)
	rightCreationDate := aws.ToString(right.CreationDate)
	if leftCreationDate != rightCreationDate {
		return leftCreationDate > rightCreationDate
	}
	return aws.ToString(left.ImageId) > aws.ToString(right.ImageId)
}

func debianArchitecture(architecture ec2_types.ArchitectureValues) (string, error) {
	switch architecture {
	case ec2_types.ArchitectureValuesX8664:
		return "amd64", nil
	case ec2_types.ArchitectureValuesArm64:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported Debian architecture: %s", architecture)
	}
}
