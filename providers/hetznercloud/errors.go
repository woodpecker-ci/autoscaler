package hetznercloud

import "errors"

var (
	ErrIllegalLablePrefix   = errors.New("illegal label prefix")
	ErrImageNotFound        = errors.New("image not found")
	ErrSSHKeyNotFound       = errors.New("SSH key not found")
	ErrNetworkNotFound      = errors.New("network not found")
	ErrFirewallNotFound     = errors.New("firewall not found")
	ErrServerTypeNotFound   = errors.New("server type not found")
	ErrLocationNotSupported = errors.New("server type not available in location")
	ErrImageNotSupported    = errors.New("image not available for server type")
	ErrNoMatchingServerType = errors.New("no configured server type matches requested capability")
)
