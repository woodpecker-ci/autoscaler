package linode

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/linode/linodego"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
)

// editorconfig-checker-enable
type Provider struct {
	region        string
	name          string
	instanceType  string
	image         string
	config        *config.Config
	sshKey        string
	rootPass      string
	stackscriptID int
	tags          []string
	client        *linodego.Client
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &Provider{
		name:          "linode",
		region:        c.String("linode-region"),
		instanceType:  c.String("linode-instance-type"),
		image:         c.String("linode-image"),
		sshKey:        c.String("linode-ssh-key"),
		rootPass:      c.String("linode-root-pass"),
		stackscriptID: c.Int("linode-stackscript-id"),
		config:        config,
	}

	p.client = newClient(c.String("linode-api-token"))

	err := p.setupKeypair(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: setupKeypair: %w", p.name, err)
	}

	p.tags = c.StringSlice("linode-tags")

	return p, nil
}

func (p *Provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}
	userDataString := b64.StdEncoding.EncodeToString([]byte(userData))

	_, err = p.client.CreateInstance(ctx, linodego.InstanceCreateOptions{
		Region:         p.region,
		Type:           p.instanceType,
		Label:          agent.Name,
		Image:          p.image,
		AuthorizedKeys: []string{p.sshKey},
		RootPass:       p.rootPass,
		Tags:           p.tags,
		Metadata: &linodego.InstanceMetadataOptions{
			UserData: userDataString,
		},
	})
	if err != nil {
		return fmt.Errorf("%s: CreateInstance: %w", p.name, err)
	}

	return nil
}

func (p *Provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*linodego.Instance, error) {
	f := linodego.Filter{}
	f.AddField(linodego.Eq, "label", agent.Name)
	fStr, err := f.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("%s: getAgent: %w", p.name, err)
	}
	opts := linodego.NewListOptions(0, string(fStr))
	server, err := p.client.ListInstances(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances %w", p.name, err)
	}

	return &server[0], nil
}

func (p *Provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := p.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: getAgent %w", p.name, err)
	}

	if server == nil {
		return nil
	}

	err = p.client.DeleteInstance(ctx, server.ID)
	if err != nil {
		return fmt.Errorf("%s: DeleteInstance %w", p.name, err)
	}

	return nil
}

func (p *Provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	f := linodego.Filter{}
	f.AddField(linodego.Contains, "label", "agent")
	fStr, err := f.MarshalJSON()
	if err != nil {
		return names, err
	}
	opts := linodego.NewListOptions(0, string(fStr))
	servers, err := p.client.ListInstances(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: ListInstances %w", p.name, err)
	}

	for _, server := range servers {
		names = append(names, server.Label)
	}

	return names, nil
}

func (p *Provider) setupKeypair(ctx context.Context) error {
	res, err := p.client.ListSSHKeys(ctx, nil)
	if err != nil {
		return err
	}

	index := map[string]string{}
	for key := range res {
		index[res[key].Label] = res[key].SSHKey
	}

	// if the account has multiple keys configured try to
	// use an existing key based on naming convention.
	for _, name := range []string{"woodpecker", "id_rsa_woodpecker"} {
		fingerprint, ok := index[name]
		if !ok {
			continue
		}
		p.sshKey = fingerprint

		return nil
	}

	// if there were no matches but the account has at least
	// one keypair already created we will select the first
	// in the list.
	if len(res) > 0 {
		p.sshKey = res[0].SSHKey
		return nil
	}

	return ErrSSHKeyNotFound
}

func newClient(apiKey string) *linodego.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiKey})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetDebug(false)

	return &linodeClient
}
