// Copyright 2023 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oracle

import (
	"context"
	"fmt"

	"github.com/woodpecker-ci/autoscaler/engine/types"
)

const Name = "oracle"

type Provider struct {
	Image          string `json:"image"`
	Region         string `json:"region"`
	Tenancy        string `json:"tenancy"`
	Compartment    string `json:"compartment"`
	User           string `json:"user"`
	Key            string `json:"key"`
	Fingerprint    string `json:"fingerprint"`
	Subnet         string `json:"subnet"`
	Vcn            string `json:"vcn"`
	SecurityGroup  string `json:"security_group"`
	Shape          string `json:"shape"`
	AuthorisedKeys string `json:"authorised_keys"`
}

func (p *Provider) DeployAgent(ctx context.Context, agent *types.Agent) error {
	return fmt.Errorf("not implemented")
}

func (p *Provider) getAgents(ctx context.Context) ([]*types.Agent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *Provider) GetAgent(ctx context.Context, agentID string) (*types.Agent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *types.Agent) error {
	return fmt.Errorf("not implemented")
}

func (p *Provider) ListAgents(ctx context.Context) ([]*types.Agent, error) {
	return nil, fmt.Errorf("not implemented")
}
