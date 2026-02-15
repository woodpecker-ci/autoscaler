package oraclecloud

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name             string
	config           *config.Config
	computeClient    core.ComputeClient
	vcnClient        core.VirtualNetworkClient
	compartmentOCID  string
	shape            string
	imageOCID        string
	subnetOCIDs      []string
	sshKeys          []string
	userDataTemplate *template.Template
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	tenancyOCID := c.String("oraclecloud-tenancy-ocid")
	userOCID := c.String("oraclecloud-user-ocid")
	fingerprint := c.String("oraclecloud-fingerprint")
	privateKeyPath := c.String("oraclecloud-private-key")
	region := c.String("oraclecloud-region")
	compartmentOCID := c.String("oraclecloud-compartment-ocid")

	if tenancyOCID == "" || userOCID == "" || fingerprint == "" || privateKeyPath == "" {
		return nil, fmt.Errorf("oraclecloud-tenancy-ocid, user-ocid, fingerprint, and private-key must be set")
	}

	if region == "" {
		return nil, fmt.Errorf("oraclecloud-region must be set")
	}

	if compartmentOCID == "" {
		return nil, fmt.Errorf("oraclecloud-compartment-ocid must be set")
	}

	// Read private key
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Create OCI configuration provider
	configurationProvider := common.NewRawConfigurationProvider(
		tenancyOCID,
		userOCID,
		region,
		fingerprint,
		string(privateKey),
		nil, // private key passphrase
	)

	// Create compute client
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configurationProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	// Create VCN client
	vcnClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configurationProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create VCN client: %w", err)
	}

	p := &Provider{
		name:            "oraclecloud",
		config:          config,
		computeClient:   computeClient,
		vcnClient:       vcnClient,
		compartmentOCID: compartmentOCID,
		shape:           c.String("oraclecloud-shape"),
		imageOCID:       c.String("oraclecloud-image-ocid"),
		subnetOCIDs:     c.StringSlice("oraclecloud-subnet-ocids"),
		sshKeys:         c.StringSlice("oraclecloud-ssh-keys"),
	}

	// User data template
	if u := c.String("provider-user-data"); u != "" {
		userDataTmpl, err := template.New("user-data").Parse(u)
		if err != nil {
			return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
		}
		p.userDataTemplate = userDataTmpl
	}

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Create metadata for tags
	metadata := map[string]string{
		"Name":               agent.Name,
		engine.LabelPool:     p.config.PoolID,
		"Woodpecker-Agent":   "true",
	}

	// Create instance details
	createDetails := core.LaunchInstanceDetails{
		DisplayName:     common.String(agent.Name),
		CompartmentId:   common.String(p.compartmentOCID),
		Shape:           common.String(p.shape),
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId: common.String(p.imageOCID),
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId: common.String(p.subnetOCIDs[0]), // Use first subnet
		},
		Metadata:   metadata,
		UserData:   common.String(userData),
		FreeformTags: map[string]string{
			"Woodpecker-Agent": agent.Name,
			"Woodpecker-Pool":  p.config.PoolID,
		},
	}

	// Add SSH keys if provided
	if len(p.sshKeys) > 0 {
		createDetails.Metadata["ssh_authorized_keys"] = strings.Join(p.sshKeys, "\n")
	}

	launchRequest := core.LaunchInstanceRequest{
		LaunchInstanceDetails: createDetails,
	}

	launchResponse, err := p.computeClient.LaunchInstance(ctx, launchRequest)
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	instance := launchResponse.Instance
	log.Debug().Msgf("created instance %s (%s)", *instance.Id, *instance.DisplayName)

	// Wait for instance to become running
	log.Debug().Msgf("waiting for instance %s to become running", *instance.Id)
	for range 60 {
		getRequest := core.GetInstanceRequest{
			InstanceId: instance.Id,
		}
		getResponse, err := p.computeClient.GetInstance(ctx, getRequest)
		if err != nil {
			return fmt.Errorf("%s: GetInstance: %w", p.name, err)
		}

		if getResponse.Instance.LifecycleState == core.InstanceLifecycleStateRunning {
			return nil
		}

		if getResponse.Instance.LifecycleState == core.InstanceLifecycleStateTerminating ||
			getResponse.Instance.LifecycleState == core.InstanceLifecycleStateTerminated {
			return fmt.Errorf("instance %s terminated unexpectedly", *instance.Id)
		}

		log.Debug().Msgf("instance state: %s", getResponse.Instance.LifecycleState)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("instance %s did not become running in time", *instance.Id)
}

func (p *Provider) getInstanceByDisplayName(ctx context.Context, name string) (*core.Instance, error) {
	listRequest := core.ListInstancesRequest{
		CompartmentId: common.String(p.compartmentOCID),
		DisplayName:   common.String(name),
	}

	listResponse, err := p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, err
	}

	for _, instance := range listResponse.Items {
		if *instance.DisplayName == name {
			return &instance, nil
		}
	}

	return nil, fmt.Errorf("instance with name %s not found", name)
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getInstanceByDisplayName(ctx, agent.Name)
	if err != nil {
		return err
	}

	terminateRequest := core.TerminateInstanceRequest{
		InstanceId: instance.Id,
	}

	_, err = p.computeClient.TerminateInstance(ctx, terminateRequest)
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	log.Debug().Msgf("terminated instance %s (%s)", *instance.Id, agent.Name)
	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	log.Debug().Msgf("list deployed agent names")

	var names []string

	listRequest := core.ListInstancesRequest{
		CompartmentId: common.String(p.compartmentOCID),
	}

	listResponse, err := p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	for _, instance := range listResponse.Items {
		// Check if instance has the pool tag
		if poolID, ok := instance.FreeformTags["Woodpecker-Pool"]; ok && poolID == p.config.PoolID {
			// Check if instance is running or provisioning
			if instance.LifecycleState == core.InstanceLifecycleStateRunning ||
				instance.LifecycleState == core.InstanceLifecycleStateProvisioning {
				log.Debug().Msgf("found agent %s (state: %s)", *instance.DisplayName, instance.LifecycleState)
				names = append(names, *instance.DisplayName)
			}
		}
	}

	return names, nil
}
