package hetznercloud

import "github.com/hetznercloud/hcloud-go/v2/hcloud"

type deployCandidate struct {
	rawType    string
	location   string
	serverType *hcloud.ServerType
}
