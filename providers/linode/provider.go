package linode

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/linode/linodego"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLabelPrefix   = errors.New("illegal label prefix")
	ErrImageNotFound        = errors.New("image not found")
	ErrSSHKeyNotFound       = errors.New("SSH key not found")
	ErrInstanceTypeNotFound = errors.New("instance type not found")
)

// editorconfig-checker-enable
type provider struct {
	name          string
	config        *config.Config
	sshKey        string
	rootPass      string
	stackscriptID int
	tags          []string
	client        *linodego.Client
	// resolved config
	region       linodego.Region
	instanceType linodego.LinodeType
	image        string
}

func New(ctx context.Context, c *cli.Command, config *config.Config) (types.Provider, error) {
	p := &provider{
		name:          "linode",
		image:         c.String("linode-image"),
		sshKey:        c.String("linode-ssh-key"),
		rootPass:      c.String("linode-root-pass"),
		stackscriptID: c.Int("linode-stackscript-id"),
		config:        config,
	}

	p.client = newClient(c.String("linode-api-token"))

	// Resolve region first — instance type availability is per-region.
	if err := p.resolveRegion(ctx, c.String("linode-region")); err != nil {
		return nil, err
	}

	// Then resolve the instance type, confirming it exists.
	if err := p.resolveInstanceType(ctx, c.String("linode-instance-type")); err != nil {
		return nil, err
	}

	p.printResolvedConfig()

	if err := p.setupKeyPair(ctx); err != nil {
		return nil, fmt.Errorf("%s: setupKeyPair: %w", p.name, err)
	}

	p.tags = c.StringSlice("linode-tags")

	return p, nil
}

func (p *provider) resolveRegion(ctx context.Context, regionID string) error {
	region, err := p.client.GetRegion(ctx, regionID)
	if err != nil {
		return fmt.Errorf("%s: GetRegion %q: %w", p.name, regionID, err)
	}
	p.region = *region
	return nil
}

func (p *provider) resolveInstanceType(ctx context.Context, typeID string) error {
	linodeType, err := p.client.GetType(ctx, typeID)
	if err != nil {
		return fmt.Errorf("%s: GetType %q: %w", p.name, typeID, err)
	}
	if linodeType == nil {
		return fmt.Errorf("%w: %s", ErrInstanceTypeNotFound, typeID)
	}
	p.instanceType = *linodeType
	return nil
}

func (p *provider) printResolvedConfig() {
	log.Info().
		Str("region", p.region.ID).
		Str("label", p.region.Label).
		Str("country", p.region.Country).
		Msg("deploy region")

	log.Info().
		Str("type", p.instanceType.ID).
		Str("label", p.instanceType.Label).
		Str("class", string(p.instanceType.Class)).
		Int("vcpus", p.instanceType.VCPUs).
		Int("memory_mb", p.instanceType.Memory).
		Msg("deploy with instance type")
}

func (p *provider) DeployAgent(ctx context.Context, agent *woodpecker.Agent, cb types.Capability) error {
	if err := p.validateCapability(cb); err != nil {
		return err
	}

	userData, err := cloudinit.RenderUserDataTemplate(p.config, agent, nil)
	if err != nil {
		return fmt.Errorf("%s: cloudinit.RenderUserDataTemplate: %w", p.name, err)
	}
	userDataString := b64.StdEncoding.EncodeToString([]byte(userData))

	userdataMap := make(map[string]string)

	var metadata *linodego.InstanceMetadataOptions

	// TODO: remove once linode user-data is out of beta
	if p.stackscriptID == -1 {
		metadata = &linodego.InstanceMetadataOptions{
			UserData: userDataString,
		}
	} else {
		userdataMap["userdata"] = userDataString
	}

	_, err = p.client.CreateInstance(ctx, linodego.InstanceCreateOptions{
		Region:          p.region.ID,
		Type:            p.instanceType.ID,
		Label:           agent.Name,
		Image:           p.image,
		StackScriptID:   p.stackscriptID,
		StackScriptData: userdataMap,
		AuthorizedKeys:  []string{p.sshKey},
		RootPass:        p.rootPass,
		Tags:            p.tags,
		Metadata:        metadata,
	})
	if err != nil {
		return fmt.Errorf("%s: CreateInstance: %w", p.name, err)
	}

	return nil
}

// validateCapability checks that the requested capability is satisfiable.
// Linode is an x86-64 only platform so only linux/amd64 + docker is valid.
func (p *provider) validateCapability(cb types.Capability) error {
	if cb.Platform == "linux/amd64" && cb.Backend == types.BackendDocker {
		return nil
	}
	return fmt.Errorf("%s: instance type %s does not support requested capability platform=%s backend=%s",
		p.name, p.instanceType.ID, cb.Platform, cb.Backend)
}

func (p *provider) Capabilities(_ context.Context) ([]types.Capability, error) {
	// Linode is an x86-64 only platform; the resolved instanceType confirms it exists.
	return []types.Capability{{
		Platform: "linux/amd64",
		Backend:  types.BackendDocker,
	}}, nil
}

func (p *provider) getAgent(ctx context.Context, agent *woodpecker.Agent) (*linodego.Instance, error) {
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

func (p *provider) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
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

func (p *provider) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
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

func (p *provider) setupKeyPair(ctx context.Context) error {
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
		key, ok := index[name]
		if !ok {
			continue
		}
		p.sshKey = key
		return nil
	}

	// if there were no matches but the account has at least
	// one key-pair already created we will select the first
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
