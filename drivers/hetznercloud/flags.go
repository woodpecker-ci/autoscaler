package hetznercloud

import (
	"os"

	"github.com/urfave/cli/v2"
)

const category = "Hetzner Cloud"

var optionUserDataDefault = `
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

var DriverFlags = []cli.Flag{
	// hetzner
	&cli.StringFlag{
		Name:     "hetznercloud-api-token",
		Usage:    "hetzner cloud api token",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_API_TOKEN"},
		FilePath: os.Getenv("WOODPECKER_HETZNERCLOUD_API_TOKEN_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-location",
		Value:    "nbg1",
		Usage:    "hetzner cloud location",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_LOCATION"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-server-type",
		Value:    "cx11",
		Usage:    "hetzner cloud server type",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SERVER_TYPE"},
		Category: category,
	},
	&cli.IntFlag{
		Name:     "hetznercloud-ssh-key-id",
		Value:    -1,
		Usage:    "id of a hetzner cloud ssh key",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_SSH_KEY_ID"},
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-user-data",
		Value:    optionUserDataDefault,
		Usage:    "hetzner cloud userdata template",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_USERDATA"},
		FilePath: os.Getenv("WOODPECKER_HETZNERCLOUD_USERDATA_FILE"),
		Category: category,
	},
	&cli.StringFlag{
		Name:     "hetznercloud-image",
		Value:    "ubuntu-22.04",
		Usage:    "hetzner cloud image",
		EnvVars:  []string{"WOODPECKER_HETZNERCLOUD_IMAGE"},
		Category: category,
	},
}
