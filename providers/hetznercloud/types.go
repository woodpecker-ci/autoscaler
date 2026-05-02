package hetznercloud

import "github.com/hetznercloud/hcloud-go/v2/hcloud"

type deployCandidate struct {
	location   *hcloud.Location
	serverType *hcloud.ServerType
	image      *hcloud.Image
}
