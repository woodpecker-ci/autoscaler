package yandexcloud

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	ycsdk "github.com/yandex-cloud/go-sdk/v2"
	"github.com/yandex-cloud/go-sdk/v2/credentials"
	"github.com/yandex-cloud/go-sdk/v2/pkg/options"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/providers/yandexcloud/ycapi"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	poolIDPattern     = regexp.MustCompile(`^[a-z0-9][-_a-z0-9]{0,46}$`)
	labelKeyPattern   = regexp.MustCompile(`^[a-z][-_./\\@0-9a-z]{0,62}$`)
	labelValuePattern = regexp.MustCompile(`^[-_./\\@0-9a-z]{0,63}$`)
)

// Yandex Cloud serves VM metadata on the same address used by the cloud-init
// datasource. The route is added at the end of the rendered bootstrap script.
var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

const (
	defaultMemory = int64(4 * 1024 * 1024 * 1024)
	defaultDisk   = int64(20 * 1024 * 1024 * 1024)
	bytesPerKib   = int64(1024)
	bytesPerMib   = int64(1024 * 1024)
	bytesPerGib   = int64(1024 * 1024 * 1024)
	bytesPerKb    = int64(1000)
	bytesPerMb    = int64(1000 * 1000)
	bytesPerGb    = int64(1000 * 1000 * 1000)
	maxUserLabels = 63
)

type settings struct {
	folderID           string
	subnetID           string
	serviceAccountFile string
	iamToken           string
	instanceSA         bool
	imageID            string
	imageFamily        string
	imageFolderID      string
	platformID         string
	cores              int64
	memory             int64
	coreFraction       int64
	diskType           string
	diskSize           int64
	securityGroups     []string
	publicIPv4         bool
	labels             map[string]string
	operationTimeout   time.Duration
	credentialMode     string
}

type provider struct {
	name     string
	config   *config.Config
	settings settings
	client   ycapi.Client
	image    ycapi.Image
	subnet   ycapi.Subnet
}

func New(ctx context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	settings, err := settingsFromCommand(c, cfg.PoolID)
	if err != nil {
		return nil, fmt.Errorf("yandexcloud: %w", err)
	}

	client, err := newClient(ctx, settings)
	if err != nil {
		return nil, fmt.Errorf("yandexcloud: %w", err)
	}
	return newProvider(ctx, cfg, settings, client)
}

func newClient(ctx context.Context, settings settings) (ycapi.Client, error) {
	var creds credentials.Credentials
	switch settings.credentialMode {
	case "service-account-key-file":
		var err error
		creds, err = credentials.ServiceAccountKeyFile(settings.serviceAccountFile)
		if err != nil {
			return nil, fmt.Errorf("load service account key: %w", err)
		}
	case "iam-token":
		creds = credentials.IAMToken(settings.iamToken)
	case "instance-service-account":
		creds = credentials.InstanceServiceAccount()
	default:
		return nil, ErrCredentialsRequired
	}

	sdk, err := ycsdk.Build(ctx,
		options.WithCredentials(creds),
		options.WithDefaultRetryOptions(),
	)
	if err != nil {
		return nil, fmt.Errorf("build SDK: %w", err)
	}
	log.Info().Str("credential_mode", settings.credentialMode).Msg("authenticated with Yandex Cloud")
	return ycapi.NewClient(sdk), nil
}

func newProvider(ctx context.Context, cfg *config.Config, settings settings, client ycapi.Client) (types.Provider, error) {
	p := &provider{
		name:     "yandexcloud",
		config:   cfg,
		settings: settings,
		client:   client,
	}

	if !poolIDPattern.MatchString(cfg.PoolID) {
		return nil, fmt.Errorf("%w: %q", ErrPoolIDInvalid, cfg.PoolID)
	}

	if err := validateLabels(settings.labels); err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	subnet, err := client.GetSubnet(ctx, settings.subnetID)
	if err != nil {
		return nil, fmt.Errorf("%s: get subnet: %w", p.name, err)
	}
	if subnet.FolderID != "" && subnet.FolderID != settings.folderID {
		return nil, fmt.Errorf("%s: %w: got %q, want %q", p.name, ErrSubnetFolderMismatch, subnet.FolderID, settings.folderID)
	}
	if subnet.ZoneID == "" {
		return nil, fmt.Errorf("%s: subnet has no availability zone", p.name)
	}
	p.subnet = subnet

	image, err := client.GetImage(ctx, settings.imageFolderID, settings.imageID, settings.imageFamily)
	if err != nil {
		return nil, fmt.Errorf("%s: get image: %w", p.name, err)
	}
	if !strings.EqualFold(image.Status, "READY") {
		return nil, fmt.Errorf("%s: %w: status is %q", p.name, ErrImageNotReady, image.Status)
	}
	if image.ID == "" {
		return nil, fmt.Errorf("%s: image API returned an empty ID", p.name)
	}
	if settings.diskSize < image.MinDiskSize {
		return nil, fmt.Errorf("%s: disk size %d is smaller than image minimum %d", p.name, settings.diskSize, image.MinDiskSize)
	}
	p.image = image

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	labels := make(map[string]string, len(p.settings.labels)+1)
	for key, value := range p.settings.labels {
		labels[key] = value
	}
	labels[engine.LabelPool] = p.config.PoolID

	operationContext, cancel := context.WithTimeout(ctx, p.settings.operationTimeout)
	defer cancel()
	_, err = p.client.Create(operationContext, ycapi.CreateRequest{
		FolderID:         p.settings.folderID,
		Name:             agent.Name,
		ZoneID:           p.subnet.ZoneID,
		PlatformID:       p.settings.platformID,
		Cores:            p.settings.cores,
		Memory:           p.settings.memory,
		CoreFraction:     p.settings.coreFraction,
		Labels:           labels,
		UserData:         userData,
		ImageID:          p.image.ID,
		DiskType:         p.settings.diskType,
		DiskSize:         p.settings.diskSize,
		SubnetID:         p.settings.subnetID,
		SecurityGroupIDs: slices.Clone(p.settings.securityGroups),
		PublicIPv4:       p.settings.publicIPv4,
	})
	if err != nil {
		return fmt.Errorf("%s: create instance: %w", p.name, err)
	}
	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instances, err := p.client.List(ctx, p.settings.folderID)
	if err != nil {
		return fmt.Errorf("%s: list instances: %w", p.name, err)
	}

	instanceID := ""
	for _, instance := range instances {
		if instance.Name != agent.Name || instance.Labels[engine.LabelPool] != p.config.PoolID {
			continue
		}
		if instanceID != "" {
			return fmt.Errorf("%s: %w: name %q in pool %q", p.name, ErrDuplicateInstance, agent.Name, p.config.PoolID)
		}
		instanceID = instance.ID
	}
	if instanceID == "" {
		return nil
	}

	operationContext, cancel := context.WithTimeout(ctx, p.settings.operationTimeout)
	defer cancel()
	if err := p.client.Delete(operationContext, instanceID); err != nil {
		return fmt.Errorf("%s: delete instance: %w", p.name, err)
	}
	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	instances, err := p.client.List(ctx, p.settings.folderID)
	if err != nil {
		return nil, fmt.Errorf("%s: list instances: %w", p.name, err)
	}

	names := make([]string, 0, len(instances))
	for _, instance := range instances {
		if instance.Labels[engine.LabelPool] != p.config.PoolID {
			continue
		}
		if instance.Status == "DELETING" {
			continue
		}
		names = append(names, instance.Name)
	}
	return names, nil
}

func (p *provider) BillingModel() types.BillingModel {
	return types.BillingPerSecond
}

func settingsFromCommand(c *cli.Command, poolID string) (settings, error) {
	serviceAccountFile := strings.TrimSpace(c.String("yandexcloud-service-account-key-file"))
	iamToken := strings.TrimSpace(c.String("yandexcloud-iam-token"))
	instanceSA := c.Bool("yandexcloud-use-instance-service-account")
	credentialCount := 0
	if serviceAccountFile != "" {
		credentialCount++
	}
	if iamToken != "" {
		credentialCount++
	}
	if instanceSA {
		credentialCount++
	}
	if credentialCount == 0 {
		return settings{}, ErrCredentialsRequired
	}
	if credentialCount > 1 {
		return settings{}, ErrCredentialConflict
	}

	folderID := strings.TrimSpace(c.String("yandexcloud-folder-id"))
	if folderID == "" {
		return settings{}, ErrFolderIDRequired
	}
	subnetID := strings.TrimSpace(c.String("yandexcloud-subnet-id"))
	if subnetID == "" {
		return settings{}, ErrSubnetIDRequired
	}

	imageID := strings.TrimSpace(c.String("yandexcloud-image-id"))
	imageFamily := strings.TrimSpace(c.String("yandexcloud-image-family"))
	if imageID == "" && imageFamily == "" {
		return settings{}, ErrImageSelectorRequired
	}
	if imageID != "" && imageFamily != "" {
		return settings{}, ErrImageSelectorConflict
	}

	memory, err := parseSize(c.String("yandexcloud-memory"), defaultMemory)
	if err != nil {
		return settings{}, fmt.Errorf("parse memory: %w", err)
	}
	diskSize, err := parseSize(c.String("yandexcloud-disk-size"), defaultDisk)
	if err != nil {
		return settings{}, fmt.Errorf("parse disk size: %w", err)
	}
	operationTimeout, err := time.ParseDuration(c.String("yandexcloud-operation-timeout"))
	if err != nil {
		return settings{}, fmt.Errorf("parse operation timeout: %w", err)
	}
	if operationTimeout <= 0 {
		return settings{}, ErrOperationTimeoutInvalid
	}
	if !poolIDPattern.MatchString(poolID) {
		return settings{}, fmt.Errorf("%w: %q", ErrPoolIDInvalid, poolID)
	}

	labels, err := utils.SliceToMap(c.StringSlice("yandexcloud-labels"), "=")
	if err != nil {
		return settings{}, fmt.Errorf("parse labels: %w", err)
	}
	if err := utils.CheckReservedTags(c.StringSlice("yandexcloud-labels"), engine.LabelPrefix, ErrReservedLabelPrefix); err != nil {
		return settings{}, err
	}
	if err := validateLabels(labels); err != nil {
		return settings{}, err
	}

	credentialMode := "instance-service-account"
	switch {
	case serviceAccountFile != "":
		credentialMode = "service-account-key-file"
	case iamToken != "":
		credentialMode = "iam-token"
	}

	return settings{
		folderID:           folderID,
		subnetID:           subnetID,
		serviceAccountFile: serviceAccountFile,
		iamToken:           iamToken,
		instanceSA:         instanceSA,
		imageID:            imageID,
		imageFamily:        imageFamily,
		imageFolderID:      strings.TrimSpace(c.String("yandexcloud-image-folder-id")),
		platformID:         strings.TrimSpace(c.String("yandexcloud-platform-id")),
		cores:              int64(c.Int("yandexcloud-cores")),
		memory:             memory,
		coreFraction:       int64(c.Int("yandexcloud-core-fraction")),
		diskType:           strings.TrimSpace(c.String("yandexcloud-disk-type")),
		diskSize:           diskSize,
		securityGroups:     slices.Clone(c.StringSlice("yandexcloud-security-group-ids")),
		publicIPv4:         c.Bool("yandexcloud-public-ipv4-enable"),
		labels:             labels,
		operationTimeout:   operationTimeout,
		credentialMode:     credentialMode,
	}, nil
}

func validateLabels(labels map[string]string) error {
	if len(labels) > maxUserLabels {
		return fmt.Errorf("%w: at most 63 user labels are supported", ErrInvalidLabel)
	}
	for key, value := range labels {
		if !labelKeyPattern.MatchString(key) || !labelValuePattern.MatchString(value) {
			return fmt.Errorf("%w: %q=%q", ErrInvalidLabel, key, value)
		}
	}
	return nil
}

func parseSize(value string, defaultValue int64) (int64, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return defaultValue, nil
	}

	multiplier := int64(1)
	for _, sizeUnit := range []struct {
		suffix     string
		multiplier int64
	}{
		{suffix: "gib", multiplier: bytesPerGib},
		{suffix: "mib", multiplier: bytesPerMib},
		{suffix: "kib", multiplier: bytesPerKib},
		{suffix: "gb", multiplier: bytesPerGb},
		{suffix: "mb", multiplier: bytesPerMb},
		{suffix: "kb", multiplier: bytesPerKb},
		{suffix: "b", multiplier: 1},
	} {
		if strings.HasSuffix(value, sizeUnit.suffix) {
			value = strings.TrimSpace(strings.TrimSuffix(value, sizeUnit.suffix))
			multiplier = sizeUnit.multiplier
			break
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("size must be a positive integer with an optional byte suffix")
	}
	if parsed > int64(^uint64(0)>>1)/multiplier {
		return 0, fmt.Errorf("size is too large")
	}
	return parsed * multiplier, nil
}
