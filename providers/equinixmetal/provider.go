package equinixmetal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const apiBase = "https://api.equinix.com/metal/v1"

type provider struct {
	name      string
	config    *config.Config
	apiToken  string
	projectID string
	metro     string
	plan      string
	os        string
	tags      []string
	client    *http.Client
}

type deviceCreateRequest struct {
	Hostname        string   `json:"hostname"`
	Metro           string   `json:"metro"`
	Plan            string   `json:"plan"`
	OperatingSystem string   `json:"operating_system"`
	Userdata        string   `json:"userdata"`
	Tags            []string `json:"tags"`
}

type device struct {
	ID       string   `json:"id"`
	Hostname string   `json:"hostname"`
	Tags     []string `json:"tags"`
}

type deviceListResponse struct {
	Devices []device `json:"devices"`
}

func New(_ context.Context, c *cli.Command, cfg *config.Config) (types.Provider, error) {
	apiToken := c.String("equinixmetal-api-token")
	if apiToken == "" {
		return nil, fmt.Errorf("equinixmetal-api-token must be set")
	}
	projectID := c.String("equinixmetal-project-id")
	if projectID == "" {
		return nil, fmt.Errorf("equinixmetal-project-id must be set")
	}

	return &provider{
		name:      "equinixmetal",
		config:    cfg,
		apiToken:  apiToken,
		projectID: projectID,
		metro:     c.String("equinixmetal-metro"),
		plan:      c.String("equinixmetal-plan"),
		os:        c.String("equinixmetal-os"),
		tags:      c.StringSlice("equinixmetal-tags"),
		client:    &http.Client{},
	}, nil
}

func (p *provider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Auth-Token", p.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, cloudinit.RenderOption{})
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	tags := append([]string{p.config.PoolID}, p.tags...)

	req := deviceCreateRequest{
		Hostname:        agent.Name,
		Metro:           p.metro,
		Plan:            p.plan,
		OperatingSystem: p.os,
		Userdata:        userData,
		Tags:            tags,
	}

	data, status, err := p.do(ctx, http.MethodPost, "/projects/"+p.projectID+"/devices", req)
	if err != nil {
		return fmt.Errorf("%s: create device: %w", p.name, err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s: create device: unexpected status %d: %s", p.name, status, string(data))
	}

	log.Debug().Str("agent", agent.Name).Msgf("%s: device created", p.name)
	return nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	id, err := p.findDeviceID(ctx, agent.Name)
	if err != nil {
		return fmt.Errorf("%s: findDeviceID: %w", p.name, err)
	}
	if id == "" {
		log.Debug().Str("agent", agent.Name).Msgf("%s: device not found, skipping removal", p.name)
		return nil
	}

	data, status, err := p.do(ctx, http.MethodDelete, "/devices/"+id, nil)
	if err != nil {
		return fmt.Errorf("%s: delete device: %w", p.name, err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s: delete device: unexpected status %d: %s", p.name, status, string(data))
	}

	log.Debug().Str("agent", agent.Name).Msgf("%s: device removed", p.name)
	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	data, status, err := p.do(ctx, http.MethodGet, "/projects/"+p.projectID+"/devices?per_page=100", nil)
	if err != nil {
		return nil, fmt.Errorf("%s: list devices: %w", p.name, err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("%s: list devices: unexpected status %d: %s", p.name, status, string(data))
	}

	var resp deviceListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: decode devices: %w", p.name, err)
	}

	var names []string
	for _, d := range resp.Devices {
		for _, tag := range d.Tags {
			if tag == p.config.PoolID {
				names = append(names, d.Hostname)
				break
			}
		}
	}
	return names, nil
}

func (p *provider) findDeviceID(ctx context.Context, hostname string) (string, error) {
	data, status, err := p.do(ctx, http.MethodGet, "/projects/"+p.projectID+"/devices?per_page=100", nil)
	if err != nil {
		return "", fmt.Errorf("%s: list devices: %w", p.name, err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("%s: list devices: unexpected status %d: %s", p.name, status, string(data))
	}

	var resp deviceListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("%s: decode devices: %w", p.name, err)
	}

	for _, d := range resp.Devices {
		if d.Hostname == hostname {
			return d.ID, nil
		}
	}
	return "", nil
}
