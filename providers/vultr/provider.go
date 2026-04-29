package vultr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"github.com/vultr/govultr/v3"
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/autoscaler/utils"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

type provider struct {
	userDataTemplate *template.Template
	sshKeys          []string
	labels           map[string]string
	config           *config.Config
	enableIPv6       bool
	name             string
	client           *govultr.Client
	// resolved config
	region govultr.Region
	plan   govultr.Plan
	image  govultr.OS
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:       "vultr",
		enableIPv6: c.Bool("vultr-public-ipv6-enable"),
		config:     config,
	}
	oauthConfig := &oauth2.Config{}
	ts := oauthConfig.TokenSource(ctx, &oauth2.Token{AccessToken: c.String("vultr-api-token")})
	p.client = govultr.NewClient(oauth2.NewClient(ctx, ts))

	// first resolve and check config
	if err := p.resolveRegion(ctx, c.String("vultr-region")); err != nil {
		return nil, err
	}
	if err := p.resolvePlan(ctx, c.String("vultr-plan")); err != nil {
		return nil, err
	}
	if err := p.resolveImage(ctx, c.String("vultr-image")); err != nil {
		return nil, err
	}
	// log debug info
	p.printResolvedConfig()

	// if not done setup ssh key-pair
	if err := p.setupKeyPair(ctx); err != nil {
		return nil, fmt.Errorf("%s: setupKeyPair: %w", p.name, err)
	}

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.image.Name

	labels, err := utils.SliceToMap(c.StringSlice("vultr-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLabelPrefix, engine.LabelPrefix)
		}
	}
	p.labels = utils.MergeMaps(defaultLabels, p.labels)

	return p, nil
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cap types.Capability) error {
	if cap.Backend != types.BackendDocker ||
		cap.Platform != "linux/amd64" {
		return fmt.Errorf("we only support docker on linux/amd64 but %#v was requested", cap)
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, p.userDataTemplate)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}

	tags := make([]string, 0)
	for key, item := range p.labels {
		tags = append(tags, fmt.Sprintf("%s=%s", key, item))
	}

	instance, _, err := p.client.Instance.Create(ctx, &govultr.InstanceCreateReq{
		Hostname:        agent.Name,
		UserData:        base64.StdEncoding.EncodeToString([]byte(userData)),
		Plan:            p.plan.ID,
		Region:          p.region.ID,
		Label:           agent.Name,
		Tags:            tags,
		OsID:            p.image.ID,
		EnableVPC:       govultr.BoolToBoolPtr(false), // TODO: allow to use private networks
		ActivationEmail: govultr.BoolToBoolPtr(false),
		SSHKeys:         p.sshKeys,
		EnableIPv6:      &p.enableIPv6,
		Backups:         "disabled",
	})
	if err != nil {
		return fmt.Errorf("%s: Instance.Create: %w", p.name, err)
	}

	// TODO: move to provider utils and use backoff?
	log.Debug().Msgf("waiting for instance %s", instance.ID)
	for range 5 {
		agents, err := p.ListDeployedAgentNames(ctx)
		if err != nil {
			return fmt.Errorf("failed to return list for agents")
		}

		for _, a := range agents {
			if a == agent.Name {
				return nil
			}
		}

		log.Debug().Msgf("created agent not found in list yet")
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("instance did not resolve in agent list: %s", instance.ID)
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*govultr.Instance, error) {
	servers, _, _, err := p.client.Instance.List(ctx, &govultr.ListOptions{
		Label: agent.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	if len(servers) == 0 {
		return nil, nil
	}

	if len(servers) > 1 {
		return nil, fmt.Errorf("%s: getAgent: found multiple instances with label %s", p.name, agent.Name)
	}

	return &servers[0], nil
}

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}

	if server == nil {
		return nil
	}

	err = p.client.Instance.Delete(ctx, server.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}

	return nil
}

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var cursor string

	names := make([]string, 0)
	servers := make([]govultr.Instance, 0)

	for {
		listOptions := &govultr.ListOptions{
			Tag:     engine.LabelPool + "=" + p.config.PoolID,
			PerPage: 200, //nolint:mnd
			Cursor:  cursor,
		}

		newServers, meta, _, err := p.client.Instance.List(ctx, listOptions)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.name, err)
		}

		servers = append(servers, newServers...)

		if meta == nil || meta.Links == nil || meta.Links.Next == "" {
			break
		}
		cursor = meta.Links.Next
	}

	for _, server := range servers {
		names = append(names, server.Hostname)
	}

	return names, nil
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	// TODO: add native k8s and local backend (with FreeBSD) support
	return []types.Capability{{
		Platform: "linux/amd64",
		Backend:  types.BackendDocker,
	}}, nil
}
