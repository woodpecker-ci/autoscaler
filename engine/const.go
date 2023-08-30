package engine

import "fmt"

var (
	LabelPrefix                    = "wp.autoscaler/"
	LabelPool                      = fmt.Sprintf("%spool", LabelPrefix)
	LabelImage                     = fmt.Sprintf("%simage", LabelPrefix)
	CloudInitUserDataUbuntuDefault = `
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
				source: deb https://download.docker.com/linux/ubuntu $RELEASE stable
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
)
