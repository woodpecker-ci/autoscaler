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
	_, found, _ := imageAliasQuery(reference, ec2_types.ArchitectureValuesX8664)
	return found
}

func (p *provider) resolveImage(ctx context.Context, reference, region string, architecture ec2_types.ArchitectureValues) (ec2_types.Image, error) {
	if isImageAlias(reference) {
		return p.resolveImageAlias(ctx, reference, region, architecture)
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

type aliasQuery struct {
	namePattern string
	matchesName func(string) bool
}

func (p *provider) resolveImageAlias(ctx context.Context, alias, region string, architecture ec2_types.ArchitectureValues) (ec2_types.Image, error) {
	query, found, err := imageAliasQuery(alias, architecture)
	if err != nil {
		return ec2_types.Image{}, fmt.Errorf("%s: %w", p.name, err)
	}
	if !found {
		return ec2_types.Image{}, fmt.Errorf("%s: unsupported aws-ami-id alias: %s", p.name, alias)
	}
	images, err := p.client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []ec2_types.Filter{
			{Name: aws.String("architecture"), Values: []string{string(architecture)}},
			{Name: aws.String("is-public"), Values: []string{"true"}},
			{Name: aws.String("name"), Values: []string{query.namePattern}},
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
		if !isVerifiedAliasImage(image, architecture, query.matchesName) {
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

func isVerifiedAliasImage(image ec2_types.Image, architecture ec2_types.ArchitectureValues, matchesName func(string) bool) bool {
	return image.Architecture == architecture &&
		matchesName(aws.ToString(image.Name)) &&
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

func imageAliasQuery(alias string, architecture ec2_types.ArchitectureValues) (aliasQuery, bool, error) {
	debianArch, awsArch, err := imageArchitectures(architecture)
	if err != nil {
		return aliasQuery{}, true, err
	}

	if version, found := strings.CutPrefix(alias, "debian-"); found && onlyDigits(version) {
		prefix := alias + "-" + debianArch + "-"
		return aliasQuery{
			namePattern: prefix + "*",
			matchesName: func(name string) bool { return strings.HasPrefix(name, prefix) },
		}, true, nil
	}

	if version, found := strings.CutPrefix(alias, "ubuntu-"); found {
		version, found = strings.CutSuffix(version, "-server")
		if found && numericVersion(version) {
			// hvm-ssd covers both the hvm-ssd and hvm-ssd-gp3 storage generations
			prefix := "ubuntu/images/hvm-ssd"
			marker := "-" + version + "-" + debianArch + "-server-"
			return aliasQuery{
				namePattern: prefix + "*/ubuntu-*" + marker + "*",
				matchesName: func(name string) bool {
					return strings.HasPrefix(name, prefix) && strings.Contains(name, "/ubuntu-") && strings.Contains(name, marker)
				},
			}, true, nil
		}
	}

	if alias == "amazon" || alias == "amazon_linux" || alias == "amazon-linux" {
		// pinned to the current major release; bump when a new one ships
		alias = "amazon-linux-2023"
	}
	if version, found := strings.CutPrefix(alias, "amazon-linux-"); found && onlyDigits(version) {
		prefix := "al" + version + "-ami-" + version + "."
		suffix := "-" + awsArch
		return aliasQuery{
			namePattern: prefix + "*-kernel-*" + suffix,
			matchesName: func(name string) bool {
				return strings.HasPrefix(name, prefix) && strings.Contains(name, "-kernel-") && strings.HasSuffix(name, suffix)
			},
		}, true, nil
	}

	if alias == "suse" {
		// pinned to the current major release; bump when a new one ships
		alias = "suse-16"
	}
	if version, found := strings.CutPrefix(alias, "suse-"); found && onlyDigits(version) {
		prefix := "suse-sles-" + version + "-"
		suffix := "-hvm-ssd-" + awsArch
		return aliasQuery{
			namePattern: prefix + "*" + suffix,
			matchesName: func(name string) bool {
				middle, found := strings.CutPrefix(name, prefix)
				if !found {
					return false
				}
				middle, found = strings.CutSuffix(middle, suffix)
				// vDATE for 15.x service packs (sp7-v20260630) and 16.x
				// minor releases (0-v20260625); anything else is a vendor
				// variant such as sapcal, chost-byos or ecs.
				return found && suseVersion(middle)
			},
		}, true, nil
	}

	return aliasQuery{}, false, nil
}

// suseVersion reports whether the middle part of a SLES image name is a plain
// release, i.e. an optional service pack or minor release followed by the
// vDATE stamp, without any vendor variant in between.
func suseVersion(middle string) bool {
	release, rest, found := strings.Cut(middle, "-")
	if found {
		release = strings.TrimPrefix(release, "sp")
		if !onlyDigits(release) {
			return false
		}
		middle = rest
	}
	date, found := strings.CutPrefix(middle, "v")
	return found && onlyDigits(date)
}

func imageArchitectures(architecture ec2_types.ArchitectureValues) (string, string, error) {
	switch architecture {
	case ec2_types.ArchitectureValuesX8664:
		return "amd64", "x86_64", nil
	case ec2_types.ArchitectureValuesArm64:
		return "arm64", "arm64", nil
	default:
		return "", "", fmt.Errorf("unsupported image alias architecture: %s", architecture)
	}
}

func numericVersion(version string) bool {
	if version == "" || strings.HasPrefix(version, ".") || strings.HasSuffix(version, ".") {
		return false
	}
	for part := range strings.SplitSeq(version, ".") {
		if !onlyDigits(part) {
			return false
		}
	}
	return true
}

func onlyDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
