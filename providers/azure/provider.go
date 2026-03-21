package azure

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name             string
	subscriptionID   string
	resourceGroup    string
	location         string
	vmSize           string
	imagePublisher   string
	imageOffer       string
	imageSKU         string
	imageVersion     string
	adminUsername     string
	sshPublicKey     string
	nsgName          string
	subnetID         string
	tags             map[string]*string
	userDataTemplate *template.Template
	config           *config.Config
	vmClient         *armcompute.VirtualMachinesClient
	nicClient        *armnetwork.InterfacesClient
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:           "azure",
		subscriptionID: c.String("azure-subscription-id"),
		resourceGroup:  c.String("azure-resource-group"),
		location:       c.String("azure-location"),
		vmSize:         c.String("azure-vm-size"),
		imagePublisher: c.String("azure-image-publisher"),
		imageOffer:     c.String("azure-image-offer"),
		imageSKU:       c.String("azure-image-sku"),
		imageVersion:   c.String("azure-image-version"),
		adminUsername:   c.String("azure-admin-username"),
		sshPublicKey:   c.String("azure-ssh-public-key"),
		nsgName:        c.String("azure-nsg-name"),
		subnetID:       c.String("azure-subnet-id"),
		config:         cfg,
	}

	// Parse tags
	p.tags = map[string]*string{
		engine.LabelPool: to.Ptr(cfg.PoolID),
	}
	for _, tag := range c.StringSlice("azure-tags") {
		key, value, ok := strings.Cut(tag, "=")
		if ok {
			p.tags[key] = to.Ptr(value)
		}
	}

	// # TODO: Deprecated remove in v2.0
	if u := c.String("azure-user-data"); u != "" {
		log.Warn().Msg("azure-user-data is deprecated, please use provider-user-data instead")
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	// Use default Azure credential chain (env vars, managed identity, CLI, etc.)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%s: azidentity.NewDefaultAzureCredential: %w", p.name, err)
	}

	vmClient, err := armcompute.NewVirtualMachinesClient(p.subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: armcompute.NewVirtualMachinesClient: %w", p.name, err)
	}
	p.vmClient = vmClient

	nicClient, err := armnetwork.NewInterfacesClient(p.subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: armnetwork.NewInterfacesClient: %w", p.name, err)
	}
	p.nicClient = nicClient

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	nicName := agent.Name + "-nic"

	// Create NIC
	nicParams := armnetwork.Interface{
		Location: to.Ptr(p.location),
		Tags:     p.tags,
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr("ipconfig1"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
					},
				},
			},
		},
	}

	if p.subnetID != "" {
		nicParams.Properties.IPConfigurations[0].Properties.Subnet = &armnetwork.Subnet{
			ID: to.Ptr(p.subnetID),
		}
	}

	nicPoller, err := p.nicClient.BeginCreateOrUpdate(ctx, p.resourceGroup, nicName, nicParams, nil)
	if err != nil {
		return fmt.Errorf("%s: NIC.BeginCreateOrUpdate: %w", p.name, err)
	}

	nicResult, err := nicPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: NIC.PollUntilDone: %w", p.name, err)
	}

	// Build SSH configuration
	linuxConfig := &armcompute.LinuxConfiguration{
		DisablePasswordAuthentication: to.Ptr(true),
	}
	if p.sshPublicKey != "" {
		linuxConfig.SSH = &armcompute.SSHConfiguration{
			PublicKeys: []*armcompute.SSHPublicKey{
				{
					Path:    to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", p.adminUsername)),
					KeyData: to.Ptr(p.sshPublicKey),
				},
			},
		}
	}

	// Create VM
	vmParams := armcompute.VirtualMachine{
		Location: to.Ptr(p.location),
		Tags:     p.tags,
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(p.vmSize)),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					Publisher: to.Ptr(p.imagePublisher),
					Offer:     to.Ptr(p.imageOffer),
					SKU:       to.Ptr(p.imageSKU),
					Version:   to.Ptr(p.imageVersion),
				},
				OSDisk: &armcompute.OSDisk{
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
					ManagedDisk: &armcompute.ManagedDiskParameters{
						StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS),
					},
				},
			},
			OSProfile: &armcompute.OSProfile{
				ComputerName:  to.Ptr(sanitizeComputerName(agent.Name)),
				AdminUsername: to.Ptr(p.adminUsername),
				CustomData:    to.Ptr(base64.StdEncoding.EncodeToString([]byte(userData))),
				LinuxConfiguration: linuxConfig,
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
	}

	vmPoller, err := p.vmClient.BeginCreateOrUpdate(ctx, p.resourceGroup, agent.Name, vmParams, nil)
	if err != nil {
		return fmt.Errorf("%s: VM.BeginCreateOrUpdate: %w", p.name, err)
	}

	_, err = vmPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: VM.PollUntilDone: %w", p.name, err)
	}

	return nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	// Delete VM
	vmPoller, err := p.vmClient.BeginDelete(ctx, p.resourceGroup, agent.Name, nil)
	if err != nil {
		// If VM not found, nothing to do
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("%s: VM.BeginDelete: %w", p.name, err)
	}

	_, err = vmPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: VM.Delete.PollUntilDone: %w", p.name, err)
	}

	// Clean up the NIC
	nicName := agent.Name + "-nic"
	nicPoller, err := p.nicClient.BeginDelete(ctx, p.resourceGroup, nicName, nil)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("%s: NIC.BeginDelete: %w", p.name, err)
	}

	_, err = nicPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: NIC.Delete.PollUntilDone: %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
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

// sanitizeComputerName ensures the computer name meets Azure's requirements
// (max 64 chars, alphanumeric and hyphens only).
func sanitizeComputerName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	s := result.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// isNotFoundError checks if an Azure error is a 404 Not Found.
func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "ResourceNotFound") || strings.Contains(err.Error(), "NotFound")
}
