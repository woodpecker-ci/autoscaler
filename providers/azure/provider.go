package azure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// blackhole the Azure IMDS endpoint so CI steps cannot read the agent token from
// custom-data via the instance metadata service (169.254.169.254).
// ponytail: IPv4-only; Azure IMDS has no IPv6 endpoint as of 2026, so no ip6 rule needed.
var blackholeMetadataAPI = []string{
	"ip -4 route add blackhole 169.254.169.254/32",
}

type provider struct {
	name          string
	subscriptionID string
	resourceGroup  string
	location       string
	vmSize         string
	image          imageReference
	subnetID       string
	adminUsername  string
	sshPublicKey   string
	tags           map[string]*string
	config         *config.Config
	vmClient       *armcompute.VirtualMachinesClient
	nicClient      *armnetwork.InterfacesClient
	diskClient     *armcompute.DisksClient
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	p := &provider{
		name:          "azure",
		subscriptionID: c.String("azure-subscription-id"),
		resourceGroup:  c.String("azure-resource-group"),
		location:       c.String("azure-location"),
		vmSize:         c.String("azure-vm-size"),
		subnetID:       c.String("azure-subnet-id"),
		adminUsername:  c.String("azure-admin-username"),
		sshPublicKey:   c.String("azure-ssh-public-key"),
		config:         cfg,
	}

	if p.subscriptionID == "" {
		return nil, ErrSubscriptionIDNotSet
	}
	if p.resourceGroup == "" {
		return nil, ErrResourceGroupNotSet
	}
	if p.subnetID == "" {
		return nil, ErrSubnetIDNotSet
	}
	if p.sshPublicKey == "" {
		return nil, ErrSSHPublicKeyNotSet
	}

	p.image = imageReference{
		publisher: c.String("azure-image-publisher"),
		offer:     c.String("azure-image-offer"),
		sku:       c.String("azure-image-sku"),
		version:   c.String("azure-image-version"),
	}

	p.tags = map[string]*string{
		engine.LabelPool: to.Ptr(cfg.PoolID),
	}
	for _, tag := range c.StringSlice("azure-tags") {
		k, v, ok := strings.Cut(tag, "=")
		if ok {
			p.tags[k] = to.Ptr(v)
		}
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%s: azidentity.NewDefaultAzureCredential: %w", p.name, err)
	}

	p.vmClient, err = armcompute.NewVirtualMachinesClient(p.subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: armcompute.NewVirtualMachinesClient: %w", p.name, err)
	}

	p.nicClient, err = armnetwork.NewInterfacesClient(p.subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: armnetwork.NewInterfacesClient: %w", p.name, err)
	}

	p.diskClient, err = armcompute.NewDisksClient(p.subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: armcompute.NewDisksClient: %w", p.name, err)
	}

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{
		PreExec: blackholeMetadataAPI,
	})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}
	userDataB64 := base64.StdEncoding.EncodeToString([]byte(userData))

	nicName := agent.Name + "-nic"
	diskName := agent.Name + "-osdisk"

	nicPoller, err := p.nicClient.BeginCreateOrUpdate(ctx, p.resourceGroup, nicName, armnetwork.Interface{
		Location: to.Ptr(p.location),
		Tags:     p.tags,
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr("ipconfig1"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet:                    &armnetwork.Subnet{ID: to.Ptr(p.subnetID)},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("%s: NIC.BeginCreateOrUpdate: %w", p.name, err)
	}
	nicResult, err := nicPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: NIC.PollUntilDone: %w", p.name, err)
	}

	vmPoller, err := p.vmClient.BeginCreateOrUpdate(ctx, p.resourceGroup, agent.Name, armcompute.VirtualMachine{
		Location: to.Ptr(p.location),
		Tags:     p.tags,
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(p.vmSize)),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					Publisher: to.Ptr(p.image.publisher),
					Offer:     to.Ptr(p.image.offer),
					SKU:       to.Ptr(p.image.sku),
					Version:   to.Ptr(p.image.version),
				},
				OSDisk: &armcompute.OSDisk{
					Name:         to.Ptr(diskName),
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
					ManagedDisk: &armcompute.ManagedDiskParameters{
						StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS),
					},
				},
			},
			OSProfile: &armcompute.OSProfile{
				ComputerName:  to.Ptr(sanitizeComputerName(agent.Name)),
				AdminUsername: to.Ptr(p.adminUsername),
				CustomData:    to.Ptr(userDataB64),
				LinuxConfiguration: &armcompute.LinuxConfiguration{
					DisablePasswordAuthentication: to.Ptr(true),
					SSH: &armcompute.SSHConfiguration{
						PublicKeys: []*armcompute.SSHPublicKey{
							{
								Path:    to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", p.adminUsername)),
								KeyData: to.Ptr(p.sshPublicKey),
							},
						},
					},
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: nicResult.ID,
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							Primary: to.Ptr(true),
						},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("%s: VM.BeginCreateOrUpdate: %w", p.name, err)
	}
	if _, err = vmPoller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("%s: VM.PollUntilDone: %w", p.name, err)
	}

	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	// Delete VM first — this detaches the NIC and unmounts the OS disk, making
	// the subsequent deletes possible. Each step is independent: a 404 means the
	// resource is already gone, which is success.
	// ponytail: partial-failure (e.g. process killed between VM and NIC delete) leaves
	// an orphaned NIC or disk in the shared RG. Mitigation: each resource is tagged with
	// engine.LabelPool, so a future sweep can detect and delete pool-tagged NICs/disks
	// with no matching VM. Upgrade path: add a reconcile sweep in engine/autoscaler.go.
	if err := p.deleteVM(ctx, agent.Name); err != nil {
		return err
	}
	if err := p.deleteNIC(ctx, agent.Name+"-nic"); err != nil {
		return err
	}
	return p.deleteDisk(ctx, agent.Name+"-osdisk")
}

func (p *provider) deleteVM(ctx context.Context, name string) error {
	poller, err := p.vmClient.BeginDelete(ctx, p.resourceGroup, name, nil)
	if err != nil {
		if isNotFound(err) {
			log.Warn().Str("vm", name).Msg("azure: VM already absent, skipping delete")
			return nil
		}
		return fmt.Errorf("%s: VM.BeginDelete: %w", p.name, err)
	}
	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("%s: VM.Delete.PollUntilDone: %w", p.name, err)
	}
	return nil
}

func (p *provider) deleteNIC(ctx context.Context, nicName string) error {
	poller, err := p.nicClient.BeginDelete(ctx, p.resourceGroup, nicName, nil)
	if err != nil {
		if isNotFound(err) {
			log.Warn().Str("nic", nicName).Msg("azure: NIC already absent, skipping delete")
			return nil
		}
		return fmt.Errorf("%s: NIC.BeginDelete: %w", p.name, err)
	}
	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("%s: NIC.Delete.PollUntilDone: %w", p.name, err)
	}
	return nil
}

func (p *provider) deleteDisk(ctx context.Context, diskName string) error {
	poller, err := p.diskClient.BeginDelete(ctx, p.resourceGroup, diskName, nil)
	if err != nil {
		if isNotFound(err) {
			log.Warn().Str("disk", diskName).Msg("azure: disk already absent, skipping delete")
			return nil
		}
		return fmt.Errorf("%s: Disk.BeginDelete: %w", p.name, err)
	}
	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("%s: Disk.Delete.PollUntilDone: %w", p.name, err)
	}
	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	pager := p.vmClient.NewListPager(p.resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: VM.List: %w", p.name, err)
		}
		for _, vm := range page.Value {
			if vm.Tags == nil {
				continue
			}
			poolTag, ok := vm.Tags[engine.LabelPool]
			if !ok || poolTag == nil || *poolTag != p.config.PoolID {
				continue
			}
			if vm.Name != nil {
				names = append(names, *vm.Name)
			}
		}
	}

	return names, nil
}

func (p *provider) BillingModel() types.BillingModel {
	// Azure bills per-second; the zero value selects the plain idle-timeout teardown policy.
	return types.BillingPerSecond
}

// sanitizeComputerName returns a name safe for Azure Linux VMs:
// max 64 characters, only alphanumeric and hyphens.
func sanitizeComputerName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// isNotFound reports whether an Azure API error is a 404 Not Found.
func isNotFound(err error) bool {
	var re *azcore.ResponseError
	return errors.As(err, &re) && re.StatusCode == 404
}
