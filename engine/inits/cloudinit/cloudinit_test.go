package cloudinit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/autoscaler/engine/inits/cloudinit"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var testUserDataStr = `
image: {{ .Image }}
environment:
	{{- range $key, $value := .Environment }}
	- {{ $key }}={{ $value }}
	{{- end }}
`

func TestRenderUserDataTemplate(t *testing.T) {
	config := &config.Config{
		Image:       "test-image",
		GRPCAddress: "test-address",
		GRPCSecure:  false,
		Environment: map[string]string{
			"FOO": "bar",
		},
		UserData: testUserDataStr,
	}
	agent := &woodpecker.Agent{
		Token: "test-token",
	}

	userData, err := cloudinit.RenderUserDataTemplate(config, agent, cloudinit.RenderOption{})

	assert.NoError(t, err)
	assert.Contains(t, userData, "test-image")
	assert.Contains(t, userData, "bar")
	assert.Contains(t, userData, "WOODPECKER_SERVER=test-address")
	assert.Contains(t, userData, "WOODPECKER_AGENT_SECRET=test-token")
}

func TestRenderUserDataTemplate_Secure(t *testing.T) {
	config := &config.Config{
		GRPCSecure: true,
		UserData:   testUserDataStr,
	}
	agent := &woodpecker.Agent{}

	userData, err := cloudinit.RenderUserDataTemplate(config, agent, cloudinit.RenderOption{})

	assert.NoError(t, err)
	assert.Contains(t, userData, "WOODPECKER_GRPC_SECURE=true")
}

func TestRenderUserDataTemplate_Error(t *testing.T) {
	config := &config.Config{
		UserData: "{{.Missing}}",
	}
	agent := &woodpecker.Agent{}

	_, err := cloudinit.RenderUserDataTemplate(config, agent, cloudinit.RenderOption{})
	assert.Error(t, err)
}

func TestRenderUserDataTemplate_CustomCommands(t *testing.T) {
	config := &config.Config{
		GRPCAddress:       "ci.woodpecker.example:9000",
		WorkflowsPerAgent: 2,
		Image:             "docker.io/woodpeckerci/woodpecker-agent:latest",
	}
	agent := &woodpecker.Agent{Token: "geheim"}
	conf, err := cloudinit.RenderUserDataTemplate(config, agent, cloudinit.RenderOption{
		PreExec:  []string{"echo exec before docker up"},
		PostExec: []string{"echo exec after docker up"},
	})
	assert.NoError(t, err)
	// editorconfig-checker-disable
	assert.EqualValues(t, `
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
        image: docker.io/woodpeckerci/woodpecker-agent:latest
        restart: always
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock
        environment:
          - WOODPECKER_AGENT_LABELS=
          - WOODPECKER_AGENT_SECRET=geheim
          - WOODPECKER_MAX_WORKFLOWS=2
          - WOODPECKER_SERVER=ci.woodpecker.example:9000

runcmd:
  - echo exec before docker up
  - sh -xc "cd /root; docker compose up -d"
  - echo exec after docker up

final_message: "The system is finally up, after $UPTIME seconds"
`, conf)
	// editorconfig-checker-enable
}
