package vultr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vultr/govultr/v3"
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"

	"os"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine"
	"go.woodpecker-ci.org/woodpecker/v2/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

type Provider struct {
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
	p := &Provider{
		name:       "vultr",
		region:     c.String("vultr-region"),
		plan:       c.String("vultr-plan"),
		image:      c.String("vultr-image"),
		enableIPv6: c.Bool("vultr-public-ipv6-enable"),
		config:     config,
	}
	fmt.Fprintf(os.Stdout, "API Token is: %s", c.String("vultr-api-token"))
	oauthConfig := &oauth2.Config{}
	ctx := context.Background()
	ts := oauthConfig.TokenSource(ctx, &oauth2.Token{AccessToken: c.String("vultr-api-token")})
	p.client = govultr.NewClient(oauth2.NewClient(ctx, ts))

	err := p.setupKeypair(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: setupKeypair: %w", p.name, err)
	}

	userDataStr := engine.CloudInitUserDataUbuntuDefault
	if _userDataStr := c.String("vultr-user-data"); _userDataStr != "" {
		userDataStr = _userDataStr
	}
	userDataTmpl, err := template.New("user-data").Parse(userDataStr)
	if err != nil {
		return nil, fmt.Errorf("%s: template.New.Parse %w", p.name, err)
	}
	p.userData = userDataTmpl

	defaultLabels := make(map[string]string, 0)
	defaultLabels[engine.LabelPool] = p.config.PoolID
	defaultLabels[engine.LabelImage] = p.image

	labels, err := engine.SliceToMap(c.StringSlice("vultr-labels"), "=")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	for _, key := range maps.Keys(labels) {
		if strings.HasPrefix(key, engine.LabelPrefix) {
			return nil, fmt.Errorf("%s: %w: %s", p.name, ErrIllegalLablePrefix, engine.LabelPrefix)
		}
	}
	p.labels = engine.MergeMaps(defaultLabels, p.labels)

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userdataString, err := engine.RenderUserDataTemplate(p.config, agent, p.userData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", p.name, err)
	}

	image := -1
	osList, _, _, err := p.client.OS.List(ctx, &govultr.ListOptions{})
	if err != nil {
		return fmt.Errorf("%s: OS.List: %w", p.name, err)
	}
	for _, osS := range osList {
		if osS.Name == p.image {
			image = osS.ID
			break
		}
	}

	tags := make([]string, 0)
	for key, item := range p.labels {
		tags = append(tags, fmt.Sprintf("%s=%s", key, item))
	}

	_, _, err = p.client.Instance.Create(ctx, &govultr.InstanceCreateReq{
		Hostname:        agent.Name,
		UserData:        base64.StdEncoding.EncodeToString([]byte(userdataString)),
		Plan:            p.plan,
		Region:          p.region,
		Label:           agent.Name,
		Tags:            tags,
		OsID:            image,
		EnableVPC:       govultr.BoolToBoolPtr(false), // TODO: allow to use private networks
		ActivationEmail: govultr.BoolToBoolPtr(false),
		SSHKeys:         p.sshKeys,
		EnableIPv6:      &p.enableIPv6,
	})
	if err != nil {
		return fmt.Errorf("%s: ServerCreate: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*govultr.Instance, error) {
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

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
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

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	listOptions := &govultr.ListOptions{
		Tag:     engine.LabelPool + "=" + string(p.config.PoolID),
		PerPage: 200,
	}

	// Allow time for records to show up in the API
	// Otherwise cleanup loop tries to destroy instances before they
	// are provisioned :(.

	// TODO: Maybe investigate whether the cleanup loop can exclude
	// newly provisioned instances for a bit?
	time.Sleep(20 * time.Second)

	servers, _, _, err := p.client.Instance.List(ctx,
		listOptions)

	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Hostname)
	}

	return names, nil
}

func (p *Provider) setupKeypair(ctx context.Context) error {
	res, _, _, err := p.client.SSHKey.List(ctx, nil)

	if err != nil {
		return err
	}

	index := map[string]string{}
	for key := range res {
		index[res[key].Name] = res[key].ID
	}

	// if the account has multiple keys configured try to
	// use an existing key based on naming convention.
	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		fingerprint, ok := index[name]
		if !ok {
			continue
		}
		p.sshKeys = append(p.sshKeys, fingerprint)

		return nil
	}

	// if there were no matches but the account has at least
	// one keypair already created we will select the first
	// in the list.
	if len(res) > 0 {
		p.sshKeys = append(p.sshKeys, res[0].ID)
		return nil
	}
	// "No matching keys"

	return errors.New("no matching keys")
}
