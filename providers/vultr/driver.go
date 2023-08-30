package vultr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/urfave/cli/v2"
	"github.com/vultr/govultr/v3"
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

type Driver struct {
	plan       string
	userData   *template.Template
	image      string
	sshKeys    []string
	labels     map[string]string
	config     *config.Config
	region     string
	enableIPv6 bool
	name       string
	client     *govultr.Client
}

func New(c *cli.Context, config *config.Config) (engine.Provider, error) {
	d := &Driver{
		name:       "vultr",
		region:     c.String("vultr-region"),
		plan:       c.String("vultr-plan"),
		image:      c.String("vultr-image"),
		sshKeys:    c.StringSlice("vultr-ssh-keys"),
		enableIPv6: c.Bool("vultr-public-ipv6-enable"),
		config:     config,
	}

	oauthConfig := &oauth2.Config{}
	ctx := context.Background()
	ts := oauthConfig.TokenSource(ctx, &oauth2.Token{AccessToken: c.String("vultr-api-token")})
	d.client = govultr.NewClient(oauth2.NewClient(ctx, ts))

	userDataStr := engine.CloudInitUserDataUbuntuDefault
	if _userDataStr := c.String("vultr-user-data"); _userDataStr != "" {
		userDataStr = _userDataStr
	}
	userDataTmpl, err := template.New("user-data").Parse(userDataStr)
	if err != nil {
		return nil, fmt.Errorf("%s: template.New.Parse %w", d.name, err)
	}
	d.userData = userDataTmpl

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = d.config.PoolID
	defaultLabels[engine.LabelImage] = d.image
	labels, err := engine.SliceToMap(c.StringSlice("vultr-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", d.name, ErrIllegalLablePrefix, engine.LabelPrefix)
		}
	}
	d.labels = engine.MergeMaps(defaultLabels, d.labels)

	return d, nil
}

func (d *Driver) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userdataString, err := engine.RenderUserDataTemplate(d.config, agent, d.userData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", d.name, err)
	}

	image := -1
	osList, _, _, err := d.client.OS.List(ctx, &govultr.ListOptions{})
	if err != nil {
		return fmt.Errorf("%s: OS.List: %w", d.name, err)
	}
	for _, osS := range osList {
		if osS.Name == d.image {
			image = osS.ID
			break
		}
	}

	sshKeys := make([]string, 0)
	for _, item := range d.sshKeys {
		keys, _, _, err := d.client.SSHKey.List(ctx, &govultr.ListOptions{})
		if err != nil {
			return fmt.Errorf("%s: SSHKey.List: %w", d.name, err)
		}
		if len(keys) == 0 {
			return fmt.Errorf("%s: %w: %s", d.name, ErrSSHKeyNotFound, item)
		}

		if len(keys) > 1 {
			return fmt.Errorf("%s: SSHKey.List: found multiple ssh keys with name %s", d.name, item)
		}

		sshKeys = append(sshKeys, keys[0].ID)
	}

	tags := make([]string, 0)
	for key, item := range d.labels {
		tags = append(tags, fmt.Sprintf("%s=%s", key, item))
	}

	_, _, err = d.client.Instance.Create(ctx, &govultr.InstanceCreateReq{
		Hostname:        agent.Name,
		UserData:        userdataString,
		Plan:            d.plan,
		Region:          d.region,
		Label:           agent.Name,
		Tags:            tags,
		OsID:            image,
		EnableVPC:       govultr.BoolToBoolPtr(false), // TODO: allow to use private networks
		ActivationEmail: govultr.BoolToBoolPtr(false),
		SSHKeys:         sshKeys,
		EnableIPv6:      &d.enableIPv6,
	})
	if err != nil {
		return fmt.Errorf("%s: ServerCreate: %w", d.name, err)
	}

	return nil
}

func (d *Driver) getAgent(ctx context.Context, agent *woodpecker.Agent) (*govultr.Instance, error) {
	servers, _, _, err := d.client.Instance.List(ctx, &govultr.ListOptions{
		Label: agent.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}

	if len(servers) == 0 {
		return nil, nil
	}

	if len(servers) > 1 {
		return nil, fmt.Errorf("%s: getAgent: found multiple instances with label %s", d.name, agent.Name)
	}

	return &servers[0], nil
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: %w", d.name, err)
	}

	if server == nil {
		return nil
	}

	err = d.client.Instance.Delete(ctx, server.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", d.name, err)
	}

	return nil
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	servers, _, _, err := d.client.Instance.List(ctx,
		&govultr.ListOptions{
			// ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("%s==%s", d.LabelPool, d.Config.PoolID)},
			Label: fmt.Sprintf("%s==%s", engine.LabelPool, d.config.PoolID),
		})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Hostname)
	}

	return names, nil
}
