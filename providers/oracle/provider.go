// Copyright 2024 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oracle

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider struct {
	name               string
	region             string
	compartmentOCID    string
	availabilityDomain string
	subnetOCID         string
	shape              string
	ocpus              int
	memoryGB           int
	imageOCID          string
	sshPublicKey       string
	bootVolumeSizeGB   int
	freeformTags       map[string]string
	config             *config.Config
	userDataTemplate   *template.Template
	computeClient      core.ComputeClient
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (engine.Provider, error) {
	p := &Provider{
		name:               "oracle",
		region:             c.String("oracle-region"),
		compartmentOCID:    c.String("oracle-compartment-ocid"),
		availabilityDomain: c.String("oracle-availability-domain"),
		subnetOCID:         c.String("oracle-subnet-ocid"),
		shape:              c.String("oracle-shape"),
		ocpus:              int(c.Int("oracle-ocpus")),
		memoryGB:           int(c.Int("oracle-memory-gb")),
		imageOCID:          c.String("oracle-image-ocid"),
		sshPublicKey:       c.String("oracle-ssh-public-key"),
		bootVolumeSizeGB:   int(c.Int("oracle-boot-volume-size-gb")),
		config:             config,
	}

	// Parse freeform tags
	p.freeformTags = make(map[string]string)
	p.freeformTags["woodpecker-pool"] = config.PoolID

	for _, tag := range c.StringSlice("oracle-freeform-tags") {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			p.freeformTags[parts[0]] = parts[1]
		}
	}

	// Create OCI configuration provider
	configProvider := common.NewRawConfigurationProvider(
		c.String("oracle-tenancy-ocid"),
		c.String("oracle-user-ocid"),
		p.region,
		c.String("oracle-fingerprint"),
		c.String("oracle-private-key"),
		nil,
	)

	// Create compute client
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("%s: NewComputeClientWithConfigurationProvider: %w", p.name, err)
	}
	p.computeClient = computeClient

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := engine.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: engine.RenderUserDataTemplate: %w", p.name, err)
	}

	// Base64 encode the user data for cloud-init
	userDataEncoded := base64.StdEncoding.EncodeToString([]byte(userData))

	// Build SSH authorized keys
	var sshKeys map[string]string
	if p.sshPublicKey != "" {
		sshKeys = map[string]string{
			"ssh_authorized_keys": p.sshPublicKey,
		}
	}

	// Prepare instance metadata
	metadata := map[string]string{
		"user_data": userDataEncoded,
	}
	if p.sshPublicKey != "" {
		metadata["ssh_authorized_keys"] = p.sshPublicKey
	}

	// Build shape config for flexible shapes
	var shapeConfig *core.LaunchInstanceShapeConfigDetails
	if strings.Contains(p.shape, "Flex") {
		ocpus := float32(p.ocpus)
		memoryGB := float32(p.memoryGB)
		shapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       &ocpus,
			MemoryInGBs: &memoryGB,
		}
	}

	// Build boot volume config
	bootVolumeSizeGB := int64(p.bootVolumeSizeGB)
	sourceDetails := core.InstanceSourceViaImageDetails{
		ImageId:             &p.imageOCID,
		BootVolumeSizeInGBs: &bootVolumeSizeGB,
	}

	// Create launch instance request
	launchRequest := core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			AvailabilityDomain: &p.availabilityDomain,
			CompartmentId:      &p.compartmentOCID,
			DisplayName:        &agent.Name,
			Shape:              &p.shape,
			ShapeConfig:        shapeConfig,
			SourceDetails:      sourceDetails,
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId:       &p.subnetOCID,
				AssignPublicIp: common.Bool(true),
			},
			Metadata:     metadata,
			FreeformTags: p.freeformTags,
		},
	}

	// Launch the instance
	response, err := p.computeClient.LaunchInstance(ctx, launchRequest)
	if err != nil {
		return fmt.Errorf("%s: LaunchInstance: %w", p.name, err)
	}

	log.Info().
		Str("agent", agent.Name).
		Str("instance_id", *response.Instance.Id).
		Msg("instance created")

	return nil
}

func (p *Provider) getInstance(ctx context.Context, agent *woodpecker.Agent) (*core.Instance, error) {
	// List instances in compartment
	listRequest := core.ListInstancesRequest{
		CompartmentId:  &p.compartmentOCID,
		DisplayName:    &agent.Name,
		LifecycleState: core.InstanceLifecycleStateRunning,
	}

	response, err := p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	for _, instance := range response.Items {
		if *instance.DisplayName == agent.Name {
			return &instance, nil
		}
	}

	// Also check for instances in STARTING state
	listRequest.LifecycleState = core.InstanceLifecycleStateStarting
	response, err = p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	for _, instance := range response.Items {
		if *instance.DisplayName == agent.Name {
			return &instance, nil
		}
	}

	return nil, nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	instance, err := p.getInstance(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getInstance: %w", p.name, err)
	}

	if instance == nil {
		log.Debug().Str("agent", agent.Name).Msg("instance not found, nothing to remove")
		return nil
	}

	// Terminate the instance
	terminateRequest := core.TerminateInstanceRequest{
		InstanceId:         instance.Id,
		PreserveBootVolume: common.Bool(false),
	}

	_, err = p.computeClient.TerminateInstance(ctx, terminateRequest)
	if err != nil {
		return fmt.Errorf("%s: TerminateInstance: %w", p.name, err)
	}

	log.Info().
		Str("agent", agent.Name).
		Str("instance_id", *instance.Id).
		Msg("instance terminated")

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	// List all running instances in the compartment
	listRequest := core.ListInstancesRequest{
		CompartmentId:  &p.compartmentOCID,
		LifecycleState: core.InstanceLifecycleStateRunning,
	}

	response, err := p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances: %w", p.name, err)
	}

	for _, instance := range response.Items {
		// Check if this instance belongs to our pool
		if poolID, ok := instance.FreeformTags["woodpecker-pool"]; ok && poolID == p.config.PoolID {
			names = append(names, *instance.DisplayName)
		}
	}

	// Also include instances in STARTING state
	listRequest.LifecycleState = core.InstanceLifecycleStateStarting
	response, err = p.computeClient.ListInstances(ctx, listRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances (starting): %w", p.name, err)
	}

	for _, instance := range response.Items {
		if poolID, ok := instance.FreeformTags["woodpecker-pool"]; ok && poolID == p.config.PoolID {
			names = append(names, *instance.DisplayName)
		}
	}

	return names, nil
}
