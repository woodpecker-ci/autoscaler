package linode

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"github.com/linode/linodego"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/autoscaler/engine"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

var (
	ErrIllegalLablePrefix = errors.New("illegal label prefix")
	ErrImageNotFound      = errors.New("image not found")
	ErrSSHKeyNotFound     = errors.New("SSH key not found")
	ErrNetworkNotFound    = errors.New("network not found")
	ErrFirewallNotFound   = errors.New("firewall not found")
)

// editorconfig-checker-disable
var optionUserDataDefault = `
#!/bin/bash

# Install Pre-requisites
apt-get update && apt-get install -y ca-certificates \
									 curl \
									 gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

# Add Docker sources
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update && apt-get install -y docker-ce docker-compose-plugin

systemctl enable --now docker

cat > /root/docker-compose.yml <<'EOS'
version: '3'
services:
  woodpecker-agent:
    image: {{ .Image }}
    restart: always
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      {{- range $key, $value := .Environment }}
        - {{ $key }}={{ $value }}
      {{- end }}
EOS

cd /root && docker compose up -d`

// editorconfig-checker-enable
type Driver struct {
	Region        string
	Token         string
	Name          string
	InstanceType  string
	Image         string
	Config        *config.Config
	SSHKey        string
	RootPass      string
	StackscriptID int
	PrivateIP     bool
	UserData      *template.Template
	Tags          []string
	LabelPrefix   string
	Client        *linodego.Client
}

func New(c *cli.Context, config *config.Config, name string) (engine.Provider, error) {
	d := &Driver{
		Name:          name,
		Token:         c.String("linode-api-token"),
		Region:        c.String("linode-region"),
		InstanceType:  c.String("linode-instance-type"),
		Image:         c.String("linode-image"),
		SSHKey:        c.String("linode-ssh-key"),
		RootPass:      c.String("linode-root-pass"),
		StackscriptID: c.Int("linode-stackscript-id"),
		LabelPrefix:   "wp.autoscaler/",
		Config:        config,
	}

	d.Client = newClient(d.Token)
	ctx := context.Background()
	err := d.setupKeypair(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: setupKeypair: %w", d.Name, err)
	}

	if userdata := c.String("linode-user-data"); userdata != "" {
		optionUserDataDefault = userdata
	}

	userdata, err := template.New("user-data").Parse(optionUserDataDefault)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	d.UserData = userdata

	d.Tags = c.StringSlice("linode-tags")

	return d, nil
}

func (d *Driver) DeployAgent(ctx context.Context, agent *woodpecker.Agent) error {
	userdataString, err := engine.RenderUserDataTemplate(d.Config, agent, d.UserData)
	if err != nil {
		return fmt.Errorf("%s: RenderUserDataTemplate: %w", d.Name, err)
	}

	userdataMap := make(map[string]string)
	userdataMap["userdata"] = b64.StdEncoding.EncodeToString([]byte(userdataString))

	_, err = d.Client.CreateInstance(ctx, linodego.InstanceCreateOptions{
		Region:          d.Region,
		Type:            d.InstanceType,
		Label:           agent.Name,
		Image:           d.Image,
		StackScriptID:   int(d.StackscriptID),
		StackScriptData: userdataMap,
		AuthorizedKeys:  []string{d.SSHKey},
		RootPass:        d.RootPass,
		Tags:            d.Tags,
	})

	if err != nil {
		return fmt.Errorf("%s: ServerCreate: %w", d.Name, err)
	}

	return nil
}

func (d *Driver) getAgent(ctx context.Context, agent *woodpecker.Agent) (*linodego.Instance, error) {
	f := linodego.Filter{}
	f.AddField(linodego.Eq, "label", agent.Name)
	fStr, err := f.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("%s: getAgent: %w", d.Name, err)
	}
	opts := linodego.NewListOptions(0, string(fStr))
	server, err := d.Client.ListInstances(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	return &server[0], nil
}

func (d *Driver) RemoveAgent(ctx context.Context, agent *woodpecker.Agent) error {
	server, err := d.getAgent(ctx, agent)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	if server == nil {
		return nil
	}

	err = d.Client.DeleteInstance(ctx, server.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Name, err)
	}

	return nil
}

func (d *Driver) ListDeployedAgentNames(ctx context.Context) ([]string, error) {
	var names []string

	f := linodego.Filter{}
	f.AddField(linodego.Contains, "label", "agent")
	fStr, err := f.MarshalJSON()
	if err != nil {
		log.Fatal(err)
	}
	opts := linodego.NewListOptions(0, string(fStr))
	servers, err := d.Client.ListInstances(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Name, err)
	}

	for _, server := range servers {
		names = append(names, server.Label)
	}

	return names, nil
}

func (d *Driver) setupKeypair(ctx context.Context) error {
	res, err := d.Client.ListSSHKeys(ctx, nil)
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
		d.SSHKey = fingerprint

		return nil
	}

	// if there were no matches but the account has at least
	// one keypair already created we will select the first
	// in the list.
	if len(res) > 0 {
		d.SSHKey = res[0].SSHKey
		return nil
	}
	// "No matching keys"

	return errors.New("no matching keys")
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
