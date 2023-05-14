package provider

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/woodpecker-ci/autoscaler/config"
	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"
)

type Provider interface {
	Setup() error
	DeployAgent(context.Context, *woodpecker.Agent) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
	ListDeployedAgentNames(context.Context) ([]string, error)
}

var userDataTemplate = `
#cloud-config

apt_reboot_if_required: false
package_update: false
package_upgrade: false

groups:
  - docker

system_info:
  default_user:
    groups: [ docker ]

apt:
  sources:
    docker.list:
      source: deb [arch=amd64] https://download.docker.com/linux/ubuntu $RELEASE stable
      keyid: 0EBFCD88

packages:
  - docker-ce
  - docker-compose-plugin

write_files:
- path: /root/docker-compose.yml
  content: |
    # docker-compose.yml
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

runcmd:
  - sh -xc "cd /root; docker compose up -d"

final_message: "The system is finally up, after $UPTIME seconds"
`

func getUserDataTemplate(config *config.Config, agent *woodpecker.Agent) (string, error) {
	tmpl, err := template.New("user-data").Parse(userDataTemplate)
	if err != nil {
		return "", err
	}

	params := struct {
		Image       string
		Environment map[string]string
	}{
		Image: config.Image,
		Environment: map[string]string{
			"WOODPECKER_SERVER":        config.GRPCAddress,
			"WOODPECKER_AGENT_SECRET":  agent.Token,
			"WOODPECKER_MAX_WORKFLOWS": fmt.Sprintf("%d", config.WorkflowsPerAgent),
		},
	}

	if config.GRPCSecure {
		params.Environment["WOODPECKER_GRPC_SECURE"] = "true"
	}

	for key, value := range config.Environment {
		params.Environment[key] = value
	}

	var userData bytes.Buffer
	if err := tmpl.Execute(&userData, params); err != nil {
		return "", err
	}

	return userData.String(), nil
}
