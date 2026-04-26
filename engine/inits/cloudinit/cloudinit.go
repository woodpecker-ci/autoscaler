package cloudinit

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// RenderUserDataTemplate renders the user data template for an Agent
// using the provided configuration.
func RenderUserDataTemplate(config *config.Config, agent *woodpecker.Agent, tmpl *template.Template) (string, error) {
	var err error

	switch {
	case tmpl != nil:
	case config.UserData != "":
		tmpl, err = template.New("user-data").Parse(config.UserData)
	default:
		tmpl, err = template.New("user-data").Parse(CloudInitUserDataUbuntuDefault)
	}

	if err != nil {
		return "", fmt.Errorf("template.New.Parse %w", err)
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

	params.Environment["WOODPECKER_AGENT_LABELS"] = genExtraAgentLabels(config.ExtraAgentLabels)

	var userData bytes.Buffer
	if err := tmpl.Execute(&userData, params); err != nil {
		return "", err
	}

	return userData.String(), nil
}

func genExtraAgentLabels(conf map[string]string) string {
	out := make([]string, 0, len(conf))
	for k, v := range conf {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(out, ",")
}

// editorconfig-checker-disable
var CloudInitUserDataUbuntuDefault = `
#cloud-config

package_reboot_if_required: false
package_update: true
package_upgrade: false

groups:
  - docker

system_info:
  default_user:
    groups: [ docker ]

apt:
  sources:
    docker.list:
      keyid: 9DC858229FC7DD38854AE2D88D81803C0EBFCD88
      keyserver: https://download.docker.com/linux/ubuntu/gpg
      source: deb [signed-by=$KEY_FILE] https://download.docker.com/linux/ubuntu $RELEASE stable

packages:
  - docker-ce
  - docker-compose-plugin
  - binfmt-support
  - qemu-user-static

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
` // editorconfig-checker-enable
