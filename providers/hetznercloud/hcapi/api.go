package hcapi

import (
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Client interface {
	Firewall() FirewallClient
	Image() ImageClient
	Network() NetworkClient
	Server() ServerClient
	ServerType() ServerTypeClient
	SSHKey() SSHKeyClient
}

type client struct {
	client *hcloud.Client
}

type ServerTypeClient interface {
	hcloud.IServerTypeClient
}

type serverTypeClient struct {
	hcloud.IServerTypeClient
}

type ImageClient interface {
	hcloud.IImageClient
}

type imageClient struct {
	hcloud.IImageClient
}

type SSHKeyClient interface {
	hcloud.ISSHKeyClient
}

type sshKeyClient struct {
	hcloud.ISSHKeyClient
}

type ServerClient interface {
	hcloud.IServerClient
}

type serverClient struct {
	hcloud.IServerClient
}

type NetworkClient interface {
	hcloud.INetworkClient
}

type networkClient struct {
	hcloud.INetworkClient
}

type FirewallClient interface {
	hcloud.IFirewallClient
}

type firewallClient struct {
	hcloud.IFirewallClient
}

func NewClient(opts ...hcloud.ClientOption) Client {
	return &client{
		client: hcloud.NewClient(opts...),
	}
}

func NewServerTypeClient(client hcloud.IServerTypeClient) ServerTypeClient {
	return &serverTypeClient{
		IServerTypeClient: client,
	}
}

func (c *client) ServerType() ServerTypeClient {
	return NewServerTypeClient(&c.client.ServerType)
}

func NewImageClient(client hcloud.IImageClient) ImageClient {
	return &imageClient{
		IImageClient: client,
	}
}

func (c *client) Image() ImageClient {
	return NewImageClient(&c.client.Image)
}

func NewSSHKeyClient(client hcloud.ISSHKeyClient) SSHKeyClient {
	return &sshKeyClient{
		ISSHKeyClient: client,
	}
}

func (c *client) SSHKey() SSHKeyClient {
	return NewSSHKeyClient(&c.client.SSHKey)
}

func NewServerClient(client *hcloud.ServerClient) ServerClient {
	return &serverClient{
		IServerClient: client,
	}
}

func (c *client) Server() ServerClient {
	return NewServerClient(&c.client.Server)
}

func NewFirewallClient(client hcloud.IFirewallClient) FirewallClient {
	return &firewallClient{
		IFirewallClient: client,
	}
}

func (c *client) Firewall() FirewallClient {
	return NewFirewallClient(&c.client.Firewall)
}

func NewNetworkClient(client hcloud.INetworkClient) NetworkClient {
	return &networkClient{
		INetworkClient: client,
	}
}

func (c *client) Network() NetworkClient {
	return NewNetworkClient(&c.client.Network)
}
